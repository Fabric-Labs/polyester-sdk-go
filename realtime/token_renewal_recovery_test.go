package realtime

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func subscribeTransfers(t *testing.T, srv *renewalServer, unit time.Duration) (*Subscription[models.LedgerTransfer], *errorLog) {
	t.Helper()
	c := srv.client(t)
	c.renewalUnit = unit
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sub, err := SubscribeProtoBatch(ctx, c, transfersChannel(), decode.LedgerTransferBatchFromBytes)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	log := &errorLog{}
	sub.SetOnError(log.add)
	go drainTransfers(ctx, sub, make(chan time.Time, 1))
	return sub, log
}

type errorLog struct {
	mu   sync.Mutex
	errs []error
}

func (l *errorLog) add(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, err)
}

func (l *errorLog) snapshot() []error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]error(nil), l.errs...)
}

// A renewal the token API or the server refuses is reported. The server then
// ends the subscription at expiry, the client resubscribes with a fresh token,
// says so through Resubscribes, and renews on the new connection.
func TestTransferRenewalRefusedReportsThenResubscribes(t *testing.T) {
	const unit = 100 * time.Millisecond
	cases := []struct {
		name  string
		setup func(srv *renewalServer)
		want  func(error) bool
	}{
		{
			name: "token API refuses",
			setup: func(srv *renewalServer) {
				var calls atomic.Int64
				hook := func(kind string) int {
					// The second subscription token is the first renewal.
					if kind == "sub" && calls.Add(1) == 2 {
						return http.StatusForbidden
					}
					return http.StatusOK
				}
				srv.tokenHook.Store(&hook)
			},
			want: func(err error) bool {
				var authErr *sdkerrors.AuthError
				return errors.As(err, &authErr) && authErr.Status == http.StatusForbidden
			},
		},
		{
			name: "server refuses",
			setup: func(srv *renewalServer) {
				var calls atomic.Int64
				hook := func(kind string) renewalAction {
					if kind == "sub_refresh" && calls.Add(1) == 1 {
						return refuseRenewal
					}
					return answerRenewal
				}
				srv.onCommand.Store(&hook)
			},
			want: func(err error) bool {
				return strings.Contains(err.Error(), "subscription token renewal refused: centrifugo error 109")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newRenewalServer(t, renewalServerConfig{unit: unit, subTTL: 2})
			tc.setup(srv)
			sub, log := subscribeTransfers(t, srv, unit)

			hardening.WaitUntil(t, func() bool { return sub.Resubscribes() == 1 && srv.subRenew.Load() >= 2 }, 5*time.Second)
			if got := srv.subExpiry.Load(); got != 1 {
				t.Errorf("server ended the subscription %d times, want once for the refused renewal", got)
			}
			if got := srv.accepted.Load(); got != 2 {
				t.Errorf("client opened %d connections, want 2", got)
			}
			errs := log.snapshot()
			if len(errs) == 0 || !tc.want(errs[0]) {
				t.Fatalf("first reported error = %v, want the refused renewal", errs)
			}
			var ended *serverUnsubscribedError
			if len(errs) < 2 || !errors.As(errs[1], &ended) || ended.code != 2501 {
				t.Errorf("reported errors = %v, want the server's 2501 after the refusal", errs)
			}
			if sub.Err() != nil {
				t.Errorf("subscription failed: %v", sub.Err())
			}
		})
	}
}

// A connection the server drops reconnects with fresh tokens and renews them
// on the new connection; nothing from the old connection reaches it.
func TestTransferRenewalContinuesAfterDisconnect(t *testing.T) {
	const unit = 100 * time.Millisecond
	srv := newRenewalServer(t, renewalServerConfig{unit: unit, connTTL: 2, subTTL: 1})
	sub, _ := subscribeTransfers(t, srv, unit)

	hardening.WaitUntil(t, func() bool { return srv.subRenew.Load() >= 1 }, 5*time.Second)
	if dropped := srv.dropAll(); dropped != 1 {
		t.Fatalf("dropped %d connections", dropped)
	}
	hardening.WaitUntil(t, func() bool { return srv.subscribe.Load() == 2 }, 5*time.Second)
	renewed, refreshed := srv.subRenew.Load(), srv.refresh.Load()
	hardening.WaitUntil(t, func() bool {
		return srv.subRenew.Load() >= renewed+2 && srv.refresh.Load() >= refreshed+1
	}, 5*time.Second)
	if got := sub.Resubscribes(); got != 1 {
		t.Errorf("Resubscribes() = %d, want 1 for the dropped connection", got)
	}
	if got := srv.subExpiry.Load() + srv.conExpiry.Load(); got != 0 {
		t.Errorf("%d tokens expired", got)
	}
	if got := srv.reused.Load() + srv.stale.Load(); got != 0 {
		t.Errorf("client sent %d used or stale tokens", got)
	}
}

// lockedRand is a rand shared by the server's handlers.
type lockedRand struct {
	mu sync.Mutex
	r  *rand.Rand
}

func (l *lockedRand) duration(maxDuration time.Duration) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return time.Duration(l.r.Int64N(int64(maxDuration) + 1))
}

// Renewals race reconnects: token fetches and renewal answers take random
// time, and the server drops the connection at random moments, so a drop
// lands before, during or after a renewal. Whatever the order, a healthy
// client never lets a token expire, never sends a token twice or on a
// connection it was not fetched for, reconnects only for the drops, and stops
// renewing once closed. RENEWAL_SEED replays a run.
func TestTokenRenewalInterleavesWithReconnects(t *testing.T) {
	seed := uint64(time.Now().UnixNano())
	if env := os.Getenv("RENEWAL_SEED"); env != "" {
		parsed, err := strconv.ParseUint(env, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		seed = parsed
	}
	t.Logf("RENEWAL_SEED=%d", seed)
	rng := rand.New(rand.NewPCG(seed, 0))
	for i := range 8 {
		iterationSeed := rng.Uint64()
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Logf("iteration seed %d", iterationSeed)
			runRenewalInterleaving(t, iterationSeed)
		})
	}
}

func runRenewalInterleaving(t *testing.T, seed uint64) {
	const unit = 40 * time.Millisecond
	r := rand.New(rand.NewPCG(seed, 1))
	shared := &lockedRand{r: rand.New(rand.NewPCG(seed, 2))}
	cfg := renewalServerConfig{
		unit: unit,
		// Grace covers the slowest token fetch plus the slowest answer.
		grace:   3 * unit,
		connTTL: uint32(r.IntN(3)),
		subTTL:  uint32(1 + r.IntN(2)),
	}
	srv := newRenewalServer(t, cfg)
	tokenHook := func(string) int {
		time.Sleep(shared.duration(unit))
		return http.StatusOK
	}
	srv.tokenHook.Store(&tokenHook)
	commandHook := func(string) renewalAction {
		time.Sleep(shared.duration(unit / 2))
		return answerRenewal
	}
	srv.onCommand.Store(&commandHook)
	sub, _ := subscribeTransfers(t, srv, unit)

	drops := 1 + r.IntN(2)
	for drop := range drops {
		time.Sleep(time.Duration(r.Int64N(int64(4 * unit))))
		if dropped := srv.dropAll(); dropped != 1 {
			t.Fatalf("drop %d closed %d connections", drop, dropped)
		}
		hardening.WaitUntil(t, func() bool { return srv.subscribe.Load() == int64(drop+2) }, 5*time.Second)
	}
	// Each renewal waits a TTL plus the injected delays; two more of each
	// show renewal running on the last connection.
	renewed, refreshed := srv.subRenew.Load(), srv.refresh.Load()
	hardening.WaitUntil(t, func() bool {
		return srv.subRenew.Load() >= renewed+2 && (cfg.connTTL == 0 || srv.refresh.Load() >= refreshed+2)
	}, 5*time.Second)

	if got := srv.subExpiry.Load() + srv.conExpiry.Load(); got != 0 {
		t.Errorf("%d tokens expired on a healthy client (config %+v)", got, cfg)
	}
	if got := srv.reused.Load() + srv.stale.Load(); got != 0 {
		t.Errorf("client sent %d used or stale tokens", got)
	}
	if got := srv.accepted.Load(); got != int64(drops+1) {
		t.Errorf("client opened %d connections for %d drops", got, drops)
	}
	if got := sub.Resubscribes(); got != uint64(drops) {
		t.Errorf("Resubscribes() = %d, want %d", got, drops)
	}
	if sub.Err() != nil {
		t.Errorf("subscription failed: %v", sub.Err())
	}

	sub.Close()
	<-sub.Done()
	settled := srv.subToks.Load() + srv.connToks.Load()
	time.Sleep(4 * unit)
	if got := srv.subToks.Load() + srv.connToks.Load(); got != settled {
		t.Errorf("client fetched %d tokens after Close", got-settled)
	}
}
