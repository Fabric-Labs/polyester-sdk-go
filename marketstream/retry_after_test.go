package marketstream

import (
	"errors"
	"math"
	"net/http"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// refusedAfterLoss loses the client's connection and has its next upgrade
// refused with 503 and retryAfter; it returns the refusal's Disconnected and
// when the refused upgrade was answered.
func refusedAfterLoss(t *testing.T, retryAfter string) (*fakeServer, *Client, *Disconnected, time.Time) {
	t.Helper()
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	conn := fs.accept(t)
	expect[*Connected](t, c)
	fs.refuse <- retryAfter
	conn.close()
	lost := expect[*Disconnected](t, c)
	if sdkerrors.AnswerStatus(lost.Err) != 0 {
		t.Fatalf("a lost connection has no answer: %v", lost.Err)
	}
	refused := expect[*Disconnected](t, c)
	_, at := fs.upgradeRequests()
	return fs, c, refused, at
}

func TestA503UpgradeWaitsItsRetryAfterOf30Seconds(t *testing.T) {
	fs, c, refused, _ := refusedAfterLoss(t, "30")
	if got := sdkerrors.AnswerStatus(refused.Err); got != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503: %v", got, refused.Err)
	}
	if got := sdkerrors.RetryAfter(refused.Err); got == nil || *got != 30 {
		t.Fatalf("retry after %v, want 30", got)
	}
	var realtime *sdkerrors.RealtimeError
	if !errors.As(refused.Err, &realtime) {
		t.Fatalf("%T, want a RealtimeError", refused.Err)
	}
	// The server's delay is never shortened: not to the backoff, nor to its
	// 30-second ceiling.
	if refused.Delay != 30*time.Second {
		t.Fatalf("delay %v, want 30s", refused.Delay)
	}
	before, _ := fs.upgradeRequests()
	time.Sleep(time.Second)
	if after, _ := fs.upgradeRequests(); after != before {
		t.Fatalf("%d upgrades within a second of a 30-second Retry-After", after-before)
	}
	c.Close()
}

func TestA503UpgradeWaitsItsRetryAfterOf400Milliseconds(t *testing.T) {
	fs, c, refused, at := refusedAfterLoss(t, "0.4")
	if got := sdkerrors.RetryAfter(refused.Err); got == nil || *got != 0.4 {
		t.Fatalf("retry after %v, want 0.4", got)
	}
	if refused.Delay < 400*time.Millisecond {
		t.Fatalf("delay %v, shorter than the 0.4 s asked for", refused.Delay)
	}
	conn := fs.accept(t)
	defer conn.close()
	expect[*Connected](t, c)
	if _, again := fs.upgradeRequests(); again.Sub(at) < 400*time.Millisecond {
		t.Fatalf("reconnected %v after a 0.4 s Retry-After", again.Sub(at))
	}
}

func TestReconnectWaitIsTheLongerOfBackoffAndRetryAfter(t *testing.T) {
	answer := func(seconds float64) error {
		return &sdkerrors.RealtimeError{Msg: "refused", Status: http.StatusServiceUnavailable, RetryAfter: &seconds}
	}
	ms := time.Millisecond
	for _, tc := range []struct {
		name    string
		backoff time.Duration
		err     error
		want    time.Duration
	}{
		{"no answer", 300 * ms, errors.New("connection reset"), 300 * ms},
		{"answer without Retry-After", 300 * ms, &sdkerrors.RealtimeError{Msg: "refused", Status: 503}, 300 * ms},
		{"Retry-After 0.4 s over a shorter backoff", 300 * ms, answer(0.4), 400 * ms},
		{"backoff over a shorter Retry-After", 800 * ms, answer(0.4), 800 * ms},
		{"Retry-After 30 s", 900 * ms, answer(30), 30 * time.Second},
		{"Retry-After past the backoff's ceiling", 30 * time.Second, answer(45), 45 * time.Second},
		{"an absurd Retry-After", 300 * ms, answer(1e300), time.Duration(math.MaxInt64)},
	} {
		if got := reconnectWait(tc.backoff, tc.err); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
