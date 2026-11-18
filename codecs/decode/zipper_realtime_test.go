package decode_test

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"google.golang.org/protobuf/proto"
)

func TestZippedAssetSupplyBatchFromBytes(t *testing.T) {
	msg := &zipperv1.ZippedAssetSupplyBatch{
		Updates: []*zipperv1.ZippedAssetSupplyUpdate{{
			ZippedAssetId: 42,
			SupplyQ:       1_500_000_000,
			Status:        zipperv1.SupplyStatus_STALE,
			ObservedAt:    timestamppb.New(time.Unix(1_763_208_000, 500_000_000)),
		}, {
			ZippedAssetId: 43,
			SupplyQ:       7,
		}},
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := decode.ZippedAssetSupplyBatchFromBytes(payload, func(uint32) (int, bool) {
		return 9, true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Updates) != 2 || batch.Updates[0].ZippedAssetID != 42 {
		t.Fatalf("batch=%+v", batch)
	}
	stale := batch.Updates[0]
	if stale.Supply != "1.5" || stale.Status != "stale" || stale.ObservedAt == nil || !stale.ObservedAt.Equal(time.Unix(1_763_208_000, 500_000_000)) {
		t.Fatalf("stale update = %+v", stale)
	}
	// An update without a known status names no supply, never zero or the
	// supply_q it carries.
	if unknown := batch.Updates[1]; unknown.Supply != "" || unknown.Status != "" || unknown.ObservedAt != nil {
		t.Fatalf("update without a status = %+v", unknown)
	}
}
