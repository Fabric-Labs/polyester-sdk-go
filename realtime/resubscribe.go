package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"google.golang.org/protobuf/proto"
)

const (
	subaccountsChannelPrefix = "private:auth:subaccounts:"
	subaccountsListPath      = "/v1/auth/subaccounts"
	subaccountsListLabel     = "list sub-accounts"
	maxSubaccountListBytes   = 4 * 1024 * 1024
	accessReadTimeout        = 10 * time.Second
	// A server unsubscribe code at or above this asks the client to subscribe
	// again; a lower one ends the subscription.
	unsubscribeTemporaryFrom = 2500
)

// subaccountChannelPrefixes are the private channels a sub-account's access
// governs. The ID follows, then an optional ":proto".
var subaccountChannelPrefixes = []string{
	"private:ledger:balances:",
	"private:ledger:transfers:",
	"private:spot:trades:",
	"private:spot:triggers:events:",
	"private:spot:triggers:",
	"private:chain:lifecycle:flows:",
}

// resubscribeTiming bounds resuming subscriptions the server ended.
type resubscribeTiming struct {
	// maxAttempts is the hard limit on attempts to resume without the
	// subscription staying up for stableAfter.
	maxAttempts int
	// initialBackoff is the first per-subscription delay; it doubles per attempt.
	initialBackoff time.Duration
	// stableAfter is how long a resumed subscription must stay up to earn a
	// fresh attempt budget.
	stableAfter time.Duration
	// attemptSpacing paces resume attempts across one client, each costing two
	// token requests.
	attemptSpacing time.Duration
	// readInterval paces ListSubaccounts across one client.
	readInterval time.Duration
}

var defaultResubscribeTiming = resubscribeTiming{
	maxAttempts:    6,
	initialBackoff: 500 * time.Millisecond,
	stableAfter:    time.Minute,
	attemptSpacing: 200 * time.Millisecond,
	readInterval:   time.Second,
}

// serverUnsubscribedError ends one connection whose subscription the server ended.
type serverUnsubscribedError struct {
	code   uint32
	reason string
}

func (e *serverUnsubscribedError) Error() string {
	return fmt.Sprintf("server ended realtime subscription (code %d: %s)", e.code, e.reason)
}

func (e *serverUnsubscribedError) temporary() bool { return e.code >= unsubscribeTemporaryFrom }

func (e *serverUnsubscribedError) stop(channel string, reason sdkerrors.SubscriptionEndReason, cause error) error {
	return &sdkerrors.SubscriptionEndedError{Channel: channel, Code: e.code, ServerReason: e.reason, Reason: reason, Cause: cause}
}

// subaccountChannelID returns the sub-account ID of a channel its access governs.
func subaccountChannelID(channel string) (uint64, bool) {
	for _, prefix := range subaccountChannelPrefixes {
		rest, ok := strings.CutPrefix(channel, prefix)
		if !ok {
			continue
		}
		rest = strings.TrimSuffix(rest, ":proto")
		if rest == "" || strings.Contains(rest, ":") {
			return 0, false
		}
		id, err := codecs.IDToInt(rest, "account_id")
		if err != nil {
			return 0, false
		}
		return id, true
	}
	return 0, false
}

// resubscribeAttempt is one subscription's bounded budget for resuming.
type resubscribeAttempt struct {
	ended    *serverUnsubscribedError
	backoff  reconnectBackoff
	attempts int
	ready    bool
	readyAt  time.Time
	last     error
}

func (a *resubscribeAttempt) noteReady(now time.Time) {
	a.ready = true
	a.readyAt = now
	a.last = nil
}

// note keeps the last failure of an attempt, not the unsubscribe itself.
func (a *resubscribeAttempt) note(err error) {
	var ended *serverUnsubscribedError
	if !errors.As(err, &ended) {
		a.last = err
	}
}

// accessRead is one shared ListSubaccounts request, started after every caller
// that joined it asked.
type accessRead struct {
	done   chan struct{}
	listed map[uint64]struct{}
	err    error
}

// subaccountAccess holds what one client knows about sub-account access and
// paces the requests that ask the server.
type subaccountAccess struct {
	client *Client
	timing resubscribeTiming

	mu            sync.Mutex
	removed       map[uint64]struct{}
	next          *accessRead
	lastRead      time.Time
	nextAttemptAt time.Time
}

func newSubaccountAccess(client *Client, timing resubscribeTiming) *subaccountAccess {
	return &subaccountAccess{client: client, timing: timing, removed: map[uint64]struct{}{}}
}

// attemptAfter returns the budget for a new server unsubscribe: the current one
// unless the resumed subscription stayed up long enough to earn a new one.
func (a *subaccountAccess) attemptAfter(current *resubscribeAttempt, ended *serverUnsubscribedError, now time.Time) *resubscribeAttempt {
	if current == nil || (current.ready && now.Sub(current.readyAt) >= a.timing.stableAfter) {
		current = &resubscribeAttempt{backoff: reconnectBackoff{initial: a.timing.initialBackoff}}
	}
	current.ended = ended
	current.ready = false
	return current
}

// noteUpdate applies one auth.v1.Subaccount update, in arrival order.
func (a *subaccountAccess) noteUpdate(payload []byte) {
	var update authv1.Subaccount
	if err := proto.Unmarshal(payload, &update); err != nil || update.GetId() == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if update.GetRemoved() {
		a.removed[update.GetId()] = struct{}{}
	} else {
		delete(a.removed, update.GetId())
	}
}

func (a *subaccountAccess) seenRemoved(id uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, removed := a.removed[id]
	return removed
}

// awaitResume waits until channel may be subscribed again, within the bounded
// budget. It returns the error that ends the subscription, or nil to subscribe;
// a canceled ctx returns nil and the caller's loop exits.
func (a *subaccountAccess) awaitResume(ctx context.Context, channel string, attempt *resubscribeAttempt) error {
	id, ok := subaccountChannelID(channel)
	if !ok || a.client.credentials == nil || a.client.apiURL == "" {
		return attempt.ended.stop(channel, sdkerrors.SubscriptionEndNotResubscribable, nil)
	}
	for {
		if a.seenRemoved(id) {
			return attempt.ended.stop(channel, sdkerrors.SubscriptionEndAccessRemoved, nil)
		}
		if attempt.attempts >= a.timing.maxAttempts {
			return attempt.ended.stop(channel, sdkerrors.SubscriptionEndRetriesExhausted, attempt.last)
		}
		attempt.attempts++
		if !sleepContext(ctx, attempt.backoff.next()) {
			return nil
		}
		if a.seenRemoved(id) {
			return attempt.ended.stop(channel, sdkerrors.SubscriptionEndAccessRemoved, nil)
		}
		listed, err := a.read(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			var refused *sdkerrors.AuthError
			if errors.As(err, &refused) {
				return attempt.ended.stop(channel, sdkerrors.SubscriptionEndListRefused, err)
			}
			attempt.last = err
			continue
		}
		if _, ok := listed[id]; !ok {
			return attempt.ended.stop(channel, sdkerrors.SubscriptionEndNotListed, nil)
		}
		sleepContext(ctx, a.reserveAttempt(time.Now()))
		return nil
	}
}

// reserveAttempt takes the client's next attempt slot and returns the wait for it.
func (a *subaccountAccess) reserveAttempt(now time.Time) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	slot := a.nextAttemptAt
	if slot.Before(now) {
		slot = now
	}
	a.nextAttemptAt = slot.Add(a.timing.attemptSpacing)
	return slot.Sub(now)
}

// read returns the sub-accounts listed for this key by a request started after
// the call, shared with concurrent callers and paced across the client.
func (a *subaccountAccess) read(ctx context.Context) (map[uint64]struct{}, error) {
	a.mu.Lock()
	call := a.next
	if call == nil {
		call = &accessRead{done: make(chan struct{})}
		a.next = call
		go a.runRead(call)
	}
	a.mu.Unlock()
	select {
	case <-call.done:
		return call.listed, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *subaccountAccess) runRead(call *accessRead) {
	defer close(call.done)
	a.mu.Lock()
	wait := time.Until(a.lastRead.Add(a.timing.readInterval))
	a.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
	a.mu.Lock()
	// Callers from here on need a read that starts after them.
	a.next = nil
	a.lastRead = time.Now()
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), accessReadTimeout)
	defer cancel()
	call.listed, call.err = fetchListedSubaccounts(ctx, a.client)
}

// fetchListedSubaccounts reads the sub-accounts this key can access with a role.
func fetchListedSubaccounts(ctx context.Context, c *Client) (map[uint64]struct{}, error) {
	rawURL := strings.TrimRight(c.apiURL, "/") + subaccountsListPath
	body, err := signedGet(ctx, c.http, c.credentials, rawURL, subaccountsListLabel, maxSubaccountListBytes)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Subaccounts []struct {
			ID      any  `json:"id"`
			Role    any  `json:"role"`
			Removed bool `json:"removed"`
		} `json:"subaccounts"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, &sdkerrors.RealtimeError{Msg: subaccountsListLabel + ": invalid response"}
	}
	listed := make(map[uint64]struct{}, len(payload.Subaccounts))
	for _, entry := range payload.Subaccounts {
		id, err := listedSubaccountID(entry.ID)
		if err != nil {
			return nil, &sdkerrors.RealtimeError{Msg: subaccountsListLabel + ": invalid sub-account id"}
		}
		if !entry.Removed && hasSubaccountRole(entry.Role) {
			listed[id] = struct{}{}
		}
	}
	return listed, nil
}

func listedSubaccountID(value any) (uint64, error) {
	switch id := value.(type) {
	case string:
		// The list reports IDs in decimal; a decimal with no zero digit also
		// reads as base58, so decimal comes first.
		if parsed, err := strconv.ParseUint(id, 10, 64); err == nil {
			return parsed, nil
		}
		return codecs.IDToInt(id, "id")
	case json.Number:
		return strconv.ParseUint(id.String(), 10, 64)
	default:
		return 0, errors.New("missing id")
	}
}

func hasSubaccountRole(role any) bool {
	switch value := role.(type) {
	case string:
		return value != "" && value != authv1.SubaccountRole_SUBACCOUNT_ROLE_UNSPECIFIED.String()
	case json.Number:
		return value.String() != "0"
	default:
		return false
	}
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
