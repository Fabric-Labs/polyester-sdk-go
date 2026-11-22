package decode

import (
	"testing"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The cancel-all, cancel-all-after and batch-cancel results carry the
// server's response time as ModifyOrderResult does: ts_ns, or ts when only
// that is sent, and 0 when the server sent neither. A cancel-all retry's
// original time is passed through as the server sent it.
func TestCancelResultsReadTheServerTimestamp(t *testing.T) {
	const tsNs = uint64(1_700_000_000_123_456_789)
	ts := &timestamppb.Timestamp{Seconds: 1_700_000_000, Nanos: 5}
	const fromTs = uint64(1_700_000_000_000_000_005)
	cases := []struct {
		name    string
		tsNs    uint64
		ts      *timestamppb.Timestamp
		want    uint64
		listing orderv1.CancelAllOrdersResponse_ListingStatus
	}{
		{"ts_ns", tsNs, nil, tsNs, orderv1.CancelAllOrdersResponse_INCLUDED},
		{"ts_ns and ts", tsNs, ts, tsNs, orderv1.CancelAllOrdersResponse_INCLUDED},
		{"ts only", 0, ts, fromTs, orderv1.CancelAllOrdersResponse_INCLUDED},
		{"retry replays the original time", tsNs, ts, tsNs, orderv1.CancelAllOrdersResponse_NOT_RETAINED_READ_OPEN_ORDERS},
		{"absent", 0, nil, 0, orderv1.CancelAllOrdersResponse_INCLUDED},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cancelAll, err := CancelAllFromProto(&orderv1.CancelAllOrdersResponse{
				Status: orderv1.CancelAllOrdersResponse_SUBMITTED, MatchedOrders: 1, SubmittedCancels: 1,
				ListingStatus: tc.listing, TsNs: tc.tsNs, Ts: tc.ts,
			})
			if err != nil || cancelAll.TsNs != tc.want {
				t.Fatalf("CancelAllOrders TsNs = %d, %v; want %d", cancelAll.TsNs, err, tc.want)
			}
			after, err := CancelAllAfterFromProto(&orderv1.CancelAllAfterResponse{
				Status: orderv1.CancelAllAfterResponse_ARMED, EffectiveTimeoutSec: 30, ExpiresAtTsNs: 42, TsNs: tc.tsNs, Ts: tc.ts,
			})
			if err != nil || after.TsNs != tc.want || after.ExpiresAtTsNs != "42" {
				t.Fatalf("CancelAllAfter = %+v, %v; want TsNs %d and its own expiry", after, err, tc.want)
			}
			batch, err := BatchCancelFromProto(&orderv1.BatchCancelOrdersResponse{TsNs: tc.tsNs, Ts: tc.ts})
			if err != nil || batch.TsNs != tc.want {
				t.Fatalf("BatchCancelOrders TsNs = %d, %v; want %d", batch.TsNs, err, tc.want)
			}
		})
	}
}
