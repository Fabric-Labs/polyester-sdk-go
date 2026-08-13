package decode

import (
	feesv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/fees/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// SpotFeeRateFromProto decodes one effective spot fee row.
func SpotFeeRateFromProto(msg *feesv1.SpotFeeRate) models.SpotFeeRate {
	if msg == nil {
		return models.SpotFeeRate{}
	}
	return models.SpotFeeRate{
		SymbolID:            msg.GetSymbolId(),
		Symbol:              msg.GetSymbol(),
		MakerFeeRatePercent: msg.GetMakerFeeRatePercent(),
		TakerFeeRatePercent: msg.GetTakerFeeRatePercent(),
		VIPTier:             msg.GetVipTier(),
	}
}

// SpotFeeRatesListFromProto decodes GetSpotFeeRatesResponse.
func SpotFeeRatesListFromProto(msg *feesv1.GetSpotFeeRatesResponse) models.SpotFeeRatesList {
	rows := msg.GetFeeRates()
	out := make([]models.SpotFeeRate, 0, len(rows))
	for _, item := range rows {
		if item == nil {
			continue
		}
		out = append(out, SpotFeeRateFromProto(item))
	}
	return models.SpotFeeRatesList{FeeRates: out}
}
