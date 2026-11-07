package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"google.golang.org/protobuf/proto"
)

const renewalAccount uint64 = 2305843009213693953

func transfersChannel() string {
	return "private:ledger:transfers:" + codecs.FormatID(renewalAccount) + ":proto"
}

func transferPublication(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&ledgerrdv1.TransferRowBatch{Events: []*ledgerrdv1.TransferRow{{AssetId: 2, TransferId: "7"}}})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// drainTransfers counts transfers until ctx ends.
func drainTransfers[T any](ctx context.Context, sub *Subscription[T], seen chan<- time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-sub.Messages():
			if !ok {
				return
			}
			select {
			case seen <- time.Now():
			default:
			}
		}
	}
}

// A transfer subscription whose subscription token lives one second stays on
// its first connection and subscription across three token lifetimes: the
// client renews the token before the server ends the subscription.
func TestTransferSubscriptionRenewsShortLivedTokenWithoutReconnect(t *testing.T) {
	srv := newRenewalServer(t, renewalServerConfig{
		unit:         time.Second,
		subTTL:       1,
		publication:  transferPublication(t),
		publishEvery: 100 * time.Millisecond,
	})
	c := srv.client(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub, err := SubscribeProtoBatch(ctx, c, transfersChannel(), decode.LedgerTransferBatchFromBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	errs := make(chan error, 16)
	sub.SetOnError(func(err error) {
		select {
		case errs <- err:
		default:
		}
	})
	seen := make(chan time.Time, 1)
	go drainTransfers(ctx, sub, seen)

	time.Sleep(3600 * time.Millisecond)
	if got := srv.subExpiry.Load(); got != 0 {
		t.Errorf("server ended the subscription %d times for an expired token", got)
	}
	if got := srv.accepted.Load(); got != 1 {
		t.Errorf("client opened %d connections, want 1", got)
	}
	if got := srv.subscribe.Load(); got != 1 {
		t.Errorf("client subscribed %d times, want 1", got)
	}
	if got := srv.subRenew.Load(); got < 3 {
		t.Errorf("client renewed the subscription token %d times in 3.6 token lifetimes, want at least 3", got)
	}
	if got := srv.reused.Load(); got != 0 {
		t.Errorf("client presented %d used or unknown tokens", got)
	}
	if got := sub.Resubscribes(); got != 0 {
		t.Errorf("Resubscribes() = %d, want 0", got)
	}
	select {
	case err := <-errs:
		t.Errorf("healthy renewal reported %v", err)
	default:
	}
	// Skip a transfer seen before the renewals; wait for one after them.
	select {
	case <-seen:
	default:
	}
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Error("no transfer delivered after the renewals")
	}
}

// A connection token that lives one second is renewed on the same connection.
func TestPrivateConnectionRenewsShortLivedTokenWithoutReconnect(t *testing.T) {
	srv := newRenewalServer(t, renewalServerConfig{unit: time.Second, connTTL: 1})
	c := srv.client(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub, err := SubscribeProtoBatch(ctx, c, transfersChannel(), decode.LedgerTransferBatchFromBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	time.Sleep(3600 * time.Millisecond)
	if got := srv.conExpiry.Load(); got != 0 {
		t.Errorf("server closed %d connections for an expired token", got)
	}
	if got := srv.accepted.Load(); got != 1 {
		t.Errorf("client opened %d connections, want 1", got)
	}
	if got := srv.refresh.Load(); got < 3 {
		t.Errorf("client renewed the connection token %d times in 3.6 token lifetimes, want at least 3", got)
	}
	if got := srv.reused.Load(); got != 0 {
		t.Errorf("client presented %d used or unknown tokens", got)
	}
}
