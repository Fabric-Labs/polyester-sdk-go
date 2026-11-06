package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	overview "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
)

func TestReadOverviewOpaqueToken(t *testing.T) {
	for _, token := range []string{"eyJ2IjoxfQ==", "000123", ""} {
		raw := &overview.ListMarketOverviewResponse{Markets: []*overview.MarketOverview{{SymbolId: 1}}, NextPageToken: token}
		result := decode.MarketOverviewListFromProto(raw, nil)
		if len(result.Markets) != 1 {
			t.Fatal(result)
		}
		if result.NextPageToken != token {
			t.Fatalf("token lost: %+v", result)
		}
	}
}
