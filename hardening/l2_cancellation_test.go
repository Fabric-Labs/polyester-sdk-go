package hardening_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

// hangingAPI answers its first `unavailable` reads with 503, then holds every
// read open until the client goes away.
type hangingAPI struct {
	unavailable atomic.Int64
	calls       atomic.Int64
	arrived     chan struct{}
	once        atomic.Bool
}

func newHangingAPI(unavailable int64) *hangingAPI {
	api := &hangingAPI{arrived: make(chan struct{})}
	api.unavailable.Store(unavailable)
	return api
}

func (a *hangingAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.calls.Add(1)
	if a.unavailable.Add(-1) >= 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"unavailable","message":"catching up"}`))
		return
	}
	if a.once.CompareAndSwap(false, true) {
		close(a.arrived)
	}
	<-r.Context().Done()
}

func cancellationClient(t *testing.T, handler http.Handler) *polyester.Client {
	t.Helper()
	srv := hardening.NewTestServer(handler)
	t.Cleanup(srv.Close)
	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		APIKeyID:        "ak_test",
		APIPrivateKey:   kp.SecretKeyHex,
		HydrateCatalogs: false,
		Timeout:         30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func requireCallerError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("err=%T %v, want errors.Is(err, %v)", err, err, want)
	}
	// It is still the SDK's transport error, as before.
	var transportErr *sdkerrors.TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("err=%T %v, want *TransportError", err, err)
	}
}

func cancelOnceArrived(t *testing.T, api *hangingAPI, call func(context.Context) error) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- call(ctx) }()
	select {
	case <-api.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("the read never arrived")
	}
	cancel()
	select {
	case err := <-errCh:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("the cancelled call did not return")
		return nil
	}
}

func TestL2CancelledUnaryIsContextCanceled(t *testing.T) {
	api := newHangingAPI(0)
	client := cancellationClient(t, api)
	err := cancelOnceArrived(t, api, func(ctx context.Context) error {
		_, err := client.MarketData.GetSpotConfig(ctx)
		return err
	})
	requireCallerError(t, err, context.Canceled)
}

func TestL2UnaryPastTheCallerDeadlineIsDeadlineExceeded(t *testing.T) {
	client := cancellationClient(t, newHangingAPI(0))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := client.MarketData.GetSpotConfig(ctx)
	requireCallerError(t, err, context.DeadlineExceeded)
}

func TestL2CancelledRetriedReadIsContextCanceled(t *testing.T) {
	api := newHangingAPI(2)
	client := cancellationClient(t, api)
	err := cancelOnceArrived(t, api, func(ctx context.Context) error {
		_, err := client.Triggers.Get(ctx, nil, "1", nil)
		return err
	})
	requireCallerError(t, err, context.Canceled)
	time.Sleep(200 * time.Millisecond)
	if got := api.calls.Load(); got != 3 {
		t.Fatalf("calls=%d, want 3: a cancelled read must stop retrying", got)
	}
}

func TestL2RetriedReadPastTheCallerDeadlineIsDeadlineExceeded(t *testing.T) {
	// Every read answers unavailable, so the deadline lands in a backoff sleep
	// or in a read, depending on timing.
	api := newHangingAPI(1 << 30)
	client := cancellationClient(t, api)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	_, err := client.Triggers.Get(ctx, nil, "1", nil)
	requireCallerError(t, err, context.DeadlineExceeded)
}

func TestL2ServerCanceledAnswerIsNotTheCallersCancellation(t *testing.T) {
	client := cancellationClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(499)
		_, _ = w.Write([]byte(`{"code":"canceled","message":"server gave up"}`))
	}))
	_, err := client.MarketData.GetSpotConfig(context.Background())
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("err=%T %v: a server answer is not the caller's cancellation", err, err)
	}
}

func interleavingSeed(t *testing.T) uint64 {
	if raw := os.Getenv("POLYESTER_INTERLEAVING_SEED"); raw != "" {
		seed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return seed
	}
	return rand.Uint64()
}

// A cancellation racing the answer ends as the answer or as the caller's
// cancellation, never as another error.
func TestL2CancelRacingAnAnswerIsAnAnswerOrContextCanceled(t *testing.T) {
	seed := interleavingSeed(t)
	rng := rand.New(rand.NewPCG(seed, seed))
	var serverDelay atomic.Int64
	client := cancellationClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(serverDelay.Load()))
		if strings.HasSuffix(r.URL.Path, "/GetTrigger") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":"unavailable","message":"catching up"}`))
			return
		}
		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
	}))
	for i := range 200 {
		serverDelay.Store(int64(rng.IntN(4000)) * int64(time.Microsecond))
		ctx, cancel := context.WithCancel(context.Background())
		cancelAfter := time.Duration(rng.IntN(6000)) * time.Microsecond
		timer := time.AfterFunc(cancelAfter, cancel)
		var err error
		retried := i%2 == 1
		if retried {
			_, err = client.Triggers.Get(ctx, nil, "1", nil)
		} else {
			_, err = client.MarketData.GetSpotConfig(ctx)
		}
		timer.Stop()
		cancel()
		switch {
		case retried && !errors.Is(err, context.Canceled):
			// Every read answers unavailable and the retry budget is far
			// longer than the window, so only the cancellation ends it.
			t.Fatalf("seed=%d run=%d: err=%T %v", seed, i, err, err)
		case !retried && err != nil && !errors.Is(err, context.Canceled):
			t.Fatalf("seed=%d run=%d: err=%T %v", seed, i, err, err)
		}
	}
}
