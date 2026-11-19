package transport

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"connectrpc.com/connect"

	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

// answerRecord holds the status and delay of the answer to a call's latest
// round trip, read from its headers as they arrive. connect-go reports a
// failure it meets later on that answer, a body cut short, longer than it
// reads or past its deadline, a redirect it cannot follow, or the client's
// timeout, without the answer's headers; answerInterceptor gives that error
// the recorded ones.
type answerRecord struct {
	mu       sync.Mutex
	status   int
	wireCode string
	header   http.Header
}

type answerRecordKey struct{}

// answerRecordOf returns the record of the call ctx belongs to, or nil.
func answerRecordOf(ctx context.Context) *answerRecord {
	record, _ := ctx.Value(answerRecordKey{}).(*answerRecord)
	return record
}

// clear forgets an earlier round trip's answer, such as a redirect that was
// followed.
func (r *answerRecord) clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status, r.wireCode, r.header = 0, "", nil
}

// set records a failed answer's status and the headers that name its delay.
func (r *answerRecord) set(resp *http.Response) {
	if r == nil {
		return
	}
	header := http.Header{}
	for _, name := range []string{"Retry-After", "Retry-After-Ms"} {
		if values := resp.Header.Values(name); len(values) > 0 {
			header[name] = values
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status, r.wireCode, r.header = resp.StatusCode, "", header
}

// setWireCode records the code the answer's ConnectRPC error body named, as
// the transport parsed it; empty when the body named none.
func (r *answerRecord) setWireCode(code string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wireCode = code
}

// attach gives err the recorded answer: its delay headers where the error
// lacks them, and its status and wire code in a *wire.AnswerError wrapping
// err, so wire.MapConnectError reads them whatever error it maps to and
// never from what the answer sent.
func (r *answerRecord) attach(err error) error {
	var connectErr *connect.Error
	if r == nil || !errors.As(err, &connectErr) {
		return err
	}
	r.mu.Lock()
	status, wireCode, header := r.status, r.wireCode, r.header
	r.mu.Unlock()
	if status == 0 {
		return err
	}
	meta := connectErr.Meta()
	if meta.Get("Retry-After") == "" && meta.Get("Retry-After-Ms") == "" {
		for name, values := range header {
			meta[name] = values
		}
	}
	return &wire.AnswerError{Err: err, Status: status, WireCode: wireCode}
}

// answerInterceptor gives every call a record of its answer, which the
// transport fills when the answer's headers arrive, and attaches it to the
// call's error. An error with no answer behind it, such as a refused
// connection, gets none.
type answerInterceptor struct{}

func (answerInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		record := &answerRecord{}
		resp, err := next(context.WithValue(ctx, answerRecordKey{}, record), req)
		if err != nil {
			err = record.attach(err)
		}
		return resp, err
	}
}

func (answerInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (answerInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
