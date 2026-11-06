package decode_test

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	market "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"testing"
)

func TestReadCandleOpaqueToken(t *testing.T) {
	for _, token := range []string{"eyJ2IjoxfQ==", "000123", ""} {
		result, err := decode.CandlesFromProto(&market.GetCandlesResponse{Candles: []*market.CandlePoint{{TsSec: 100}}, NextPageToken: token}, 8, 8)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Candles) != 1 {
			t.Fatal(result)
		}
		if result.NextPageToken != token {
			t.Fatalf("token %q lost: %+v", token, result)
		}
	}
}
