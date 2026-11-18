package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// scriptedTransport answers each request with the next error, then succeeds,
// recording each body it was sent.
type scriptedTransport struct {
	errs   []error
	bodies []string
}

func (s *scriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		body = string(raw)
	}
	s.bodies = append(s.bodies, body)
	if n := len(s.bodies); n <= len(s.errs) {
		return nil, s.errs[n-1]
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func newRequest(t *testing.T, ctx context.Context, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.example/orders.v1.OrdersService/CreateOrder", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return req
}

var errNotEstablished = errors.New(connNotEstablished)

func TestUnsentResendSendsTheSameBodyAgain(t *testing.T) {
	base := &scriptedTransport{errs: []error{errNotEstablished, fmt.Errorf("wrapped: %w", errNotEstablished)}}
	resp, err := unsentResendRoundTripper{base: base}.RoundTrip(newRequest(t, context.Background(), []byte("order")))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	if len(base.bodies) != 3 || base.bodies[0] != "order" || base.bodies[1] != "order" || base.bodies[2] != "order" {
		t.Fatalf("bodies=%q, want the same body three times", base.bodies)
	}
}

func TestUnsentResendIsBounded(t *testing.T) {
	errs := make([]error, maxUnsentResends+2)
	for i := range errs {
		errs[i] = errNotEstablished
	}
	base := &scriptedTransport{errs: errs}
	if _, err := (unsentResendRoundTripper{base: base}).RoundTrip(newRequest(t, context.Background(), []byte("order"))); !errors.Is(err, errNotEstablished) {
		t.Fatalf("err=%v", err)
	}
	if len(base.bodies) != maxUnsentResends+1 {
		t.Fatalf("sent %d times, want %d", len(base.bodies), maxUnsentResends+1)
	}
}

// Any other failure may have reached the server, so it is never resent.
func TestUnsentResendLeavesOtherFailuresAlone(t *testing.T) {
	for _, failure := range []error{
		io.ErrUnexpectedEOF,
		errors.New("http2: client conn not usable"),
		errors.New("write: broken pipe"),
		context.DeadlineExceeded,
	} {
		base := &scriptedTransport{errs: []error{failure}}
		if _, err := (unsentResendRoundTripper{base: base}).RoundTrip(newRequest(t, context.Background(), []byte("order"))); !errors.Is(err, failure) {
			t.Fatalf("%v: err=%v", failure, err)
		}
		if len(base.bodies) != 1 {
			t.Fatalf("%v: sent %d times, want once", failure, len(base.bodies))
		}
	}
}

func TestUnsentResendNeedsARewindableBody(t *testing.T) {
	base := &scriptedTransport{errs: []error{errNotEstablished}}
	req := newRequest(t, context.Background(), []byte("order"))
	req.GetBody = nil
	if _, err := (unsentResendRoundTripper{base: base}).RoundTrip(req); !errors.Is(err, errNotEstablished) || len(base.bodies) != 1 {
		t.Fatalf("err=%v sent=%d, want the error after one send", err, len(base.bodies))
	}
}

func TestUnsentResendStopsWhenTheCallerIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	base := &scriptedTransport{errs: []error{errNotEstablished}}
	if _, err := (unsentResendRoundTripper{base: base}).RoundTrip(newRequest(t, ctx, []byte("order"))); !errors.Is(err, errNotEstablished) || len(base.bodies) != 1 {
		t.Fatalf("err=%v sent=%d", err, len(base.bodies))
	}
}

// trackedBody records whether the wrapper closed a body it did not send.
type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

// The caller cancels while the body is rewound: nothing more is sent, and
// the rewound body is closed.
func TestUnsentResendStopsWhenTheCallerCancelsDuringTheRewind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := &scriptedTransport{errs: []error{errNotEstablished}}
	req := newRequest(t, ctx, []byte("order"))
	rewound := &trackedBody{Reader: bytes.NewReader([]byte("order"))}
	req.GetBody = func() (io.ReadCloser, error) {
		cancel()
		return rewound, nil
	}
	if _, err := (unsentResendRoundTripper{base: base}).RoundTrip(req); !errors.Is(err, errNotEstablished) && !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if len(base.bodies) != 1 || !rewound.closed {
		t.Fatalf("sent %d times (want once), rewound body closed=%v", len(base.bodies), rewound.closed)
	}
}

// The caller's deadline passes while the body is rewound: nothing more is
// sent. The rewind waits for the deadline, so it passes there whatever the
// machine's speed.
func TestUnsentResendStopsWhenTheDeadlinePassesDuringTheRewind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	base := &scriptedTransport{errs: []error{errNotEstablished}}
	req := newRequest(t, ctx, []byte("order"))
	rewound := &trackedBody{Reader: bytes.NewReader([]byte("order"))}
	req.GetBody = func() (io.ReadCloser, error) {
		<-ctx.Done()
		return rewound, nil
	}
	if _, err := (unsentResendRoundTripper{base: base}).RoundTrip(req); !errors.Is(err, errNotEstablished) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if len(base.bodies) != 1 || !rewound.closed {
		t.Fatalf("sent %d times (want once), rewound body closed=%v", len(base.bodies), rewound.closed)
	}
}
