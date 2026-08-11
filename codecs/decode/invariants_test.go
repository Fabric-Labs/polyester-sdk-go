package decode

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
)

func TestTimestampNSRejectsMillisecondShapedResponse(t *testing.T) {
	err := ValidateTimestampNS(1_700_000_000_000, "GetTrades", "ts_ns")
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError, got %T: %v", err, err)
	}
	if err := ValidateTimestampNS(1_700_000_000_000_000_000, "GetTrades", "ts_ns"); err != nil {
		t.Fatalf("nanoseconds rejected: %v", err)
	}
}

func TestMarketTradesCheckedEnforcesTimestampUnits(t *testing.T) {
	_, err := MarketTradesFromProtoChecked(&marketdatav1.GetTradesResponse{
		Trades: []*marketdatav1.MarketTrade{{TsNs: 1_700_000_000_000}},
	}, 8)
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError, got %T: %v", err, err)
	}
}
