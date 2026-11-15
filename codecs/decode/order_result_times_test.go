package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	exactTsNs = uint64(1_763_170_000_123_456_789)
	tsSeconds = int64(1_763_170_001)
	tsNanos   = int32(250_000_000)
	tsAsNs    = uint64(1_763_170_001_250_000_000)
)

func pairedTimestamp() *timestamppb.Timestamp {
	return &timestamppb.Timestamp{Seconds: tsSeconds, Nanos: tsNanos}
}

// Order write results keep the server's times: the exact *_ts_ns field, its
// paired Timestamp when only that is sent, and 0 when neither is.
func TestOrderWriteResultsKeepServerTimes(t *testing.T) {
	t.Run("create accepted_at", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			msg  *orderv1.CreateOrderResponse
			want uint64
		}{
			{"ts_ns", &orderv1.CreateOrderResponse{OrderId: 1, AcceptedAtTsNs: exactTsNs, AcceptedAt: pairedTimestamp()}, exactTsNs},
			{"timestamp only", &orderv1.CreateOrderResponse{OrderId: 1, AcceptedAt: pairedTimestamp()}, tsAsNs},
			{"unset", &orderv1.CreateOrderResponse{OrderId: 1}, 0},
		} {
			got, err := decode.OrderMutationFromProto(tc.msg)
			if err != nil {
				t.Fatal(err)
			}
			if got.AcceptedTsNs != tc.want {
				t.Fatalf("%s: AcceptedTsNs = %d, want %d", tc.name, got.AcceptedTsNs, tc.want)
			}
		}
	})
	t.Run("batch create ts", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			msg  *orderv1.BatchCreateOrdersResponse
			want uint64
		}{
			{"ts_ns", &orderv1.BatchCreateOrdersResponse{TsNs: exactTsNs, Ts: pairedTimestamp()}, exactTsNs},
			{"timestamp only", &orderv1.BatchCreateOrdersResponse{Ts: pairedTimestamp()}, tsAsNs},
			{"unset", &orderv1.BatchCreateOrdersResponse{}, 0},
		} {
			got, err := decode.BatchCreateFromProto(tc.msg)
			if err != nil {
				t.Fatal(err)
			}
			if got.TsNs != tc.want {
				t.Fatalf("%s: TsNs = %d, want %d", tc.name, got.TsNs, tc.want)
			}
		}
	})
	t.Run("modify ts", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			msg  *orderv1.ModifyOrderResponse
			want uint64
		}{
			{"ts_ns", &orderv1.ModifyOrderResponse{ActionTaken: orderv1.ModifyActionTaken_AMENDED, OldOrderId: 1, FinalOrderId: 1, TsNs: exactTsNs, Ts: pairedTimestamp()}, exactTsNs},
			{"timestamp only", &orderv1.ModifyOrderResponse{ActionTaken: orderv1.ModifyActionTaken_REPLACED, OldOrderId: 1, FinalOrderId: 2, Ts: pairedTimestamp()}, tsAsNs},
			{"unset", &orderv1.ModifyOrderResponse{ActionTaken: orderv1.ModifyActionTaken_AMENDED, OldOrderId: 1, FinalOrderId: 1}, 0},
		} {
			got, err := decode.ModifyOrderFromProto(tc.msg)
			if err != nil {
				t.Fatal(err)
			}
			if got.TsNs != tc.want {
				t.Fatalf("%s: TsNs = %d, want %d", tc.name, got.TsNs, tc.want)
			}
		}
	})
	t.Run("batch replace accepted_ts", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			msg  *orderv1.BatchReplaceOrdersResponse
			want uint64
		}{
			{"ts_ns", &orderv1.BatchReplaceOrdersResponse{BatchRequestId: 1, Status: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED, AcceptedTsNs: exactTsNs, AcceptedTs: pairedTimestamp()}, exactTsNs},
			{"timestamp only", &orderv1.BatchReplaceOrdersResponse{BatchRequestId: 1, Status: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED, AcceptedTs: pairedTimestamp()}, tsAsNs},
			{"unset", &orderv1.BatchReplaceOrdersResponse{BatchRequestId: 1, Status: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED}, 0},
		} {
			got, err := decode.BatchReplaceFromProto(tc.msg)
			if err != nil {
				t.Fatal(err)
			}
			if got.AcceptedTsNs != tc.want {
				t.Fatalf("%s: AcceptedTsNs = %d, want %d", tc.name, got.AcceptedTsNs, tc.want)
			}
		}
	})
}

// A batch replace item refused because its successor's client order ID is
// already held names the order holding it, as a batch create item does.
func TestBatchReplaceRejectionNamesExistingOrder(t *testing.T) {
	existing := uint64(77)
	got, err := decode.BatchReplaceFromProto(&orderv1.BatchReplaceOrdersResponse{
		BatchRequestId: 1,
		Status:         orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_REJECTED,
		Results: []*orderv1.BatchReplaceAdmissionItem{
			{ItemIndex: 0, Status: orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_REJECTED, OldOrderId: 42, Code: "CONFLICT_DUPLICATE_CLIENT_ORDER_ID", Error: &orderv1.ErrorDetail{
				Code: orderv1.ErrorCode_ERROR_CODE_CONFLICT_DUPLICATE_CLIENT_ORDER_ID, ExistingOrderId: &existing,
			}},
			{ItemIndex: 1, Status: orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_REJECTED, OldOrderId: 43, Code: "BAD_PRICE"},
		},
		RejectedCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Results[0].ExistingOrderID != codecs.FormatID(77) || got.Results[1].ExistingOrderID != "" {
		t.Fatalf("existing order IDs = %q, %q", got.Results[0].ExistingOrderID, got.Results[1].ExistingOrderID)
	}
}
