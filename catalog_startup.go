package polyester

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// The client loads the spot config and the funding (deposit/withdraw) config
// when it is created, each on its own. A read that fails the way a dropped or
// stalled connection does (Connect unavailable or deadline_exceeded) is read
// again, at most catalogStartupAttempts times in all, with backoff, never past
// the startup deadline, every retry through connections of its own. Anything
// else, and a response that does not validate, ends the load at once. Only
// these reads are retried here: they change nothing on the server.
const (
	catalogStartupAttempts = 3
	catalogInitialBackoff  = 250 * time.Millisecond
	catalogMaxBackoff      = time.Second
)

const (
	spotConfigOperation    = "marketdata.v1.MarketDataService/GetSpotConfig"
	fundingConfigOperation = "chain.zipper.v1.ZipperService/GetDepositWithdrawConfig"
)

// catalogSource is one startup catalog: what it is called, how to read it
// through a given HTTP client, and how to install what was read.
type catalogSource struct {
	catalog   string
	name      string
	operation string
	// read sends the read through httpClient and installs the response; a
	// *catalogInvalid error says the response did not validate.
	read func(ctx context.Context, httpClient connect.HTTPClient) error
}

// catalogInvalid marks a response that was read but did not validate.
type catalogInvalid struct{ err error }

func (e *catalogInvalid) Error() string { return e.err.Error() }
func (e *catalogInvalid) Unwrap() error { return e.err }

var catalogFailureWords = map[string]string{
	"timeout":           "timed out",
	"connection_closed": "connection closed",
	"connect_failed":    "could not connect",
	"unavailable":       "server unavailable",
	"error":             "request failed",
}

// loadCatalog reads and installs source, or returns
// *sdkerrors.CatalogHydrationError. Each attempt gets the request timeout or
// what is left before deadline, if less.
func loadCatalog(ctx context.Context, factory *transport.Factory, source catalogSource, requestTimeout time.Duration, deadline time.Time) error {
	started := time.Now()
	delay := catalogInitialBackoff
	for attempt := 1; ; attempt++ {
		refsCtx, refs := transport.WithResponseRefs(ctx)
		err := readCatalogOnce(refsCtx, factory, source, requestTimeout, deadline, attempt > 1)
		if err == nil {
			return nil
		}
		var invalid *catalogInvalid
		if stderrors.As(err, &invalid) {
			return &sdkerrors.CatalogHydrationError{
				Err:       invalid.err,
				Catalog:   source.catalog,
				Operation: source.operation,
				Failure:   "invalid",
				Attempts:  attempt,
				Msg:       fmt.Sprintf("%s (%s) is invalid: %s", source.name, source.operation, redactText(invalid.err.Error())),
			}
		}
		if ctx.Err() != nil || !catalogReadRetryable(err) || attempt >= catalogStartupAttempts || time.Now().Add(delay).After(deadline) {
			return catalogReadFailure(source, err, attempt, time.Since(started), refs.Last())
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return catalogReadFailure(source, err, attempt, time.Since(started), refs.Last())
		case <-timer.C:
		}
		delay = min(2*delay, catalogMaxBackoff)
	}
}

func readCatalogOnce(ctx context.Context, factory *transport.Factory, source catalogSource, requestTimeout time.Duration, deadline time.Time, fresh bool) error {
	attemptDeadline := time.Now().Add(requestTimeout)
	if deadline.Before(attemptDeadline) {
		attemptDeadline = deadline
	}
	ctx, cancel := context.WithDeadline(ctx, attemptDeadline)
	defer cancel()
	if !fresh {
		return source.read(ctx, factory.HTTP)
	}
	httpClient, closeConnections := factory.FreshHTTP()
	defer closeConnections()
	return source.read(ctx, httpClient)
}

// catalogReadRetryable reports a failure a dropped or stalled connection, or
// an edge that is briefly unavailable, causes. A rate refusal is not one.
func catalogReadRetryable(err error) bool {
	var rateLimited *sdkerrors.RateLimitError
	if stderrors.As(transport.MapError(err), &rateLimited) {
		return false
	}
	code := connect.CodeOf(err)
	return code == connect.CodeUnavailable || code == connect.CodeDeadlineExceeded
}

func catalogFailureKind(err error) string {
	var netErr net.Error
	switch {
	case stderrors.Is(err, context.DeadlineExceeded) || connect.CodeOf(err) == connect.CodeDeadlineExceeded:
		return "timeout"
	case stderrors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case isDialFailure(err):
		return "connect_failed"
	case stderrors.Is(err, io.EOF) || stderrors.Is(err, io.ErrUnexpectedEOF) ||
		stderrors.Is(err, syscall.ECONNRESET) || stderrors.Is(err, syscall.EPIPE) ||
		strings.Contains(err.Error(), "connection lost") ||
		strings.Contains(err.Error(), "server closed") ||
		strings.Contains(err.Error(), "use of closed network connection"):
		return "connection_closed"
	case connect.CodeOf(err) == connect.CodeUnavailable:
		return "unavailable"
	default:
		return "error"
	}
}

func isDialFailure(err error) bool {
	var opErr *net.OpError
	if stderrors.As(err, &opErr) && (opErr.Op == "dial" || opErr.Op == "proxyconnect") {
		return true
	}
	return strings.Contains(err.Error(), "proxyconnect")
}

func catalogReadFailure(source catalogSource, err error, attempts int, elapsed time.Duration, responseRef string) error {
	mapped := transport.MapError(err)
	kind := catalogFailureKind(err)
	noun := "attempts"
	if attempts == 1 {
		noun = "attempt"
	}
	msg := fmt.Sprintf("%s (%s) %s after %d %s in %.1fs", source.name, source.operation, catalogFailureWords[kind], attempts, noun, elapsed.Seconds())
	if responseRef != "" {
		msg += "; response ref " + responseRef
	}
	return &sdkerrors.CatalogHydrationError{
		Err:         mapped,
		Catalog:     source.catalog,
		Operation:   source.operation,
		Failure:     kind,
		Attempts:    attempts,
		ResponseRef: responseRef,
		Msg:         msg + ": " + redactText(mapped.Error()),
	}
}

// redactText is redact.Text: the one place text quoting a transport is redacted.
func redactText(text string) string { return redact.Text(text) }
