package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// zeroScaleCatalog mirrors the published zipper catalog shape: BONK and SHIB
// carry quantity_scale 0, USDT and BTC are nonzero-scale controls, and ETH is
// a legitimate 18-decimal asset.
func zeroScaleCatalog(t *testing.T) *catalogs.Manager {
	t.Helper()
	mgr := catalogs.NewManager()
	err := mgr.HydrateZipperConfig(models.DepositWithdrawConfig{
		Chains: []models.ZipperChainConfig{{ChainID: 1, Code: "ethereum"}},
		Assets: []models.ZipperAssetConfig{
			{Asset: "BONK", LedgerID: 45, QuantityScale: 0, Variants: []models.ZipperAssetChainVariant{{ZippedAssetID: 245, ChainID: 1}}},
			{Asset: "SHIB", LedgerID: 47, QuantityScale: 0, Variants: []models.ZipperAssetChainVariant{{ZippedAssetID: 247, ChainID: 1}}},
			{Asset: "USDT", LedgerID: 11, QuantityScale: 6, Variants: []models.ZipperAssetChainVariant{{ZippedAssetID: 11, ChainID: 1}}},
			{Asset: "BTC", LedgerID: 12, QuantityScale: 8, Variants: []models.ZipperAssetChainVariant{{ZippedAssetID: 12, ChainID: 1}}},
			{Asset: "ETH", LedgerID: 13, QuantityScale: 18, Variants: []models.ZipperAssetChainVariant{{ZippedAssetID: 13, ChainID: 1}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestZippedAssetSupplyKeepsKnownZeroScale(t *testing.T) {
	mgr := zeroScaleCatalog(t)
	msg := &zipperv1.ZippedAssetSupplyBatch{Updates: []*zipperv1.ZippedAssetSupplyUpdate{
		{ZippedAssetId: 245, SupplyQ: 3_582_802},
		{ZippedAssetId: 247, SupplyQ: 2_232_802},
		{ZippedAssetId: 11, SupplyQ: 1_500_000},
		{ZippedAssetId: 12, SupplyQ: 150_000_000},
		{ZippedAssetId: 13, SupplyQ: 1_500_000_000_000_000_000},
	}}
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := decode.ZippedAssetSupplyBatchFromBytes(payload, mgr.QuantityScaleForZippedAssetID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint32]string{245: "3582802", 247: "2232802", 11: "1.5", 12: "1.5", 13: "1.5"}
	if len(batch.Updates) != len(want) {
		t.Fatalf("updates=%+v", batch.Updates)
	}
	for _, u := range batch.Updates {
		if u.Supply != want[u.ZippedAssetID] {
			t.Errorf("zipped asset %d supply=%q want %q", u.ZippedAssetID, u.Supply, want[u.ZippedAssetID])
		}
	}
}

func TestZippedAssetSupplyUnknownScaleStillRejected(t *testing.T) {
	mgr := zeroScaleCatalog(t)
	payload, err := proto.Marshal(&zipperv1.ZippedAssetSupplyBatch{Updates: []*zipperv1.ZippedAssetSupplyUpdate{
		{ZippedAssetId: 999, SupplyQ: 3_582_802},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decode.ZippedAssetSupplyBatchFromBytes(payload, mgr.QuantityScaleForZippedAssetID); err == nil {
		t.Fatal("unknown zipped asset must not decode with a guessed scale")
	}
}
