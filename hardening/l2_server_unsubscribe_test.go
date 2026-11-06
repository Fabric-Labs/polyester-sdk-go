package hardening_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// A server-ended subscription must not sit open and silent: rows sent before
// the unsubscribe still arrive, then the channel closes with an error. A live
// control proves the same subscription stays open until the server ends it.
func TestL2ServerUnsubscribeEndsSubscriptionInsteadOfStalling(t *testing.T) {
	const channel = "public:spot:trades:1:proto"
	frames := make(chan []byte, 4)
	ws := hardening.SpawnCentrifugoFrames(nil, frames)
	t.Cleanup(ws.Close)
	rt := newRT(ws.WSURL(), "", nil, 5*time.Second)
	t.Cleanup(rt.Close)

	sub, err := realtime.SubscribeProto(context.Background(), rt, channel, identityDecode)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	var reported atomic.Int64
	sub.SetOnError(func(error) { reported.Add(1) })

	frames <- hardening.CentrifugoPublication([]byte("live"))
	select {
	case got, ok := <-sub.Messages():
		if !ok || string(got) != "live" {
			t.Fatalf("live control: got %q open=%v", got, ok)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("live control: publication not delivered")
	}
	if sub.Err() != nil {
		t.Fatalf("live control: unexpected error %v", sub.Err())
	}

	frames <- hardening.CentrifugoPublication([]byte("last"))
	frames <- hardening.CentrifugoUnsubscribe(channel, 2000, "server unsubscribe")
	var drained []string
	deadline := time.After(5 * time.Second)
	for open := true; open; {
		select {
		case got, ok := <-sub.Messages():
			if !ok {
				open = false
				break
			}
			drained = append(drained, string(got))
		case <-deadline:
			t.Fatalf("subscription stayed open and silent after the server ended it (drained %q)", drained)
		}
	}
	if len(drained) != 1 || drained[0] != "last" {
		t.Fatalf("queued rows = %q, want [last]", drained)
	}
	var ended *sdkerrors.SubscriptionEndedError
	if !errors.As(sub.Err(), &ended) || ended.Channel != channel || ended.Code != 2000 ||
		ended.ServerReason != "server unsubscribe" || ended.Reason != sdkerrors.SubscriptionEndNotResubscribable {
		t.Fatalf("server-ended subscription reported %v", sub.Err())
	}
	if reported.Load() != 1 {
		t.Fatalf("error callback ran %d times, want 1", reported.Load())
	}
}

// The unsubscribe of another channel on the socket does not end this one.
func TestL2ServerUnsubscribeOfAnotherChannelIsIgnored(t *testing.T) {
	const channel = "public:spot:trades:1:proto"
	frames := make(chan []byte, 4)
	ws := hardening.SpawnCentrifugoFrames(nil, frames)
	t.Cleanup(ws.Close)
	rt := newRT(ws.WSURL(), "", nil, 5*time.Second)
	t.Cleanup(rt.Close)
	sub, err := realtime.SubscribeProto(context.Background(), rt, channel, identityDecode)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	frames <- hardening.CentrifugoUnsubscribe("public:spot:trades:2:proto", 2000, "other")
	frames <- hardening.CentrifugoPublication([]byte("after"))
	select {
	case got, ok := <-sub.Messages():
		if !ok || string(got) != "after" {
			t.Fatalf("got %q open=%v err=%v", got, ok, sub.Err())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publication after another channel's unsubscribe not delivered")
	}
}
