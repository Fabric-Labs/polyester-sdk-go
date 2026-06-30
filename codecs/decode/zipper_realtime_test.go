package decode_test

import (
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
		}},
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := decode.ZippedAssetSupplyBatchFromBytes(payload, func(uint32) int { return 9 })
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Updates) != 1 || batch.Updates[0].ZippedAssetID != 42 {
		t.Fatalf("batch=%+v", batch)
	}
	if batch.Updates[0].Supply == "" {
		t.Fatal("expected formatted supply")
	}
}
