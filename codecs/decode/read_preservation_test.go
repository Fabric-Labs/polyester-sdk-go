package decode_test

import (
	"encoding/json"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ledger "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"reflect"
	"testing"
)

func TestReadTransferOpaqueToken(t *testing.T) {
	for _, token := range []string{"eyJ2IjoxfQ==", "000123", "0", ""} {
		t.Run(token, func(t *testing.T) {
			raw := &ledger.ListTransfersResponse{Transfers: []*ledger.TransferRow{{AssetId: 1, TsUs: 999}}, NextPageToken: token}
			for _, jsonWire := range []bool{false, true} {
				msg := &ledger.ListTransfersResponse{}
				if jsonWire {
					b, err := protojson.Marshal(raw)
					if err != nil {
						t.Fatal(err)
					}
					if err = protojson.Unmarshal(b, msg); err != nil {
						t.Fatal(err)
					}
				} else {
					b, err := proto.Marshal(raw)
					if err != nil {
						t.Fatal(err)
					}
					if err = proto.Unmarshal(b, msg); err != nil {
						t.Fatal(err)
					}
				}
				result := decode.TransfersListFromProto(msg)
				if len(result.Transfers) != 1 || result.Transfers[0].Timestamp != 999 {
					t.Fatal(result)
				}
				b, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]any
				if err = json.Unmarshal(b, &fields); err != nil {
					t.Fatal(err)
				}
				if fields["next_page_token"] != token {
					t.Fatalf("token %q lost: %s", token, b)
				}
			}
		})
	}
}

func TestReadEquityBTCPrices(t *testing.T) {
	for _, jsonWire := range []bool{false, true} {
		for _, prices := range [][]int64{{60000000000001, 61000000000009}, {}} {
			raw := &ledger.GetEquityHistorySeriesResponse{StartTsSec: 100, EndTsSec: 400, Bucket: "5m", Points: 2, Series: []*ledger.EquitySeries{{EquityQ: []int64{12345, -6789}}}, BtcPricesQ: prices}
			msg := &ledger.GetEquityHistorySeriesResponse{}
			if jsonWire {
				b, err := protojson.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
				if err = protojson.Unmarshal(b, msg); err != nil {
					t.Fatal(err)
				}
			} else {
				b, err := proto.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
				if err = proto.Unmarshal(b, msg); err != nil {
					t.Fatal(err)
				}
			}
			result := decode.EquityHistoryFromProto(msg)
			if result.Points != 2 || result.StartTsSec != 100 || result.EndTsSec != 400 || len(result.Series) != 1 || !reflect.DeepEqual(result.Series[0].EquityQ, []int64{12345, -6789}) {
				t.Fatal(result)
			}
			if !reflect.DeepEqual(result.BTCPricesQ, prices) {
				t.Fatalf("got %v want %v", result.BTCPricesQ, prices)
			}
			b, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(b, &fields); err != nil {
				t.Fatal(err)
			}
			var got []int64
			if fields["btc_prices_q"] == nil {
				t.Fatalf("BTC prices dropped: %s", b)
			}
			if err = json.Unmarshal(fields["btc_prices_q"], &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, prices) {
				t.Fatalf("got %v want %v", got, prices)
			}
		}
	}
}
