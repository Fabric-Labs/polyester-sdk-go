package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	feesv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/fees/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// SpotFeeRateFromProto decodes one effective spot fee row.
func SpotFeeRateFromProto(msg *feesv1.SpotFeeRate, cats *catalogs.Manager) models.SpotFeeRate {
	if msg == nil {
		return models.SpotFeeRate{}
	}
	return models.SpotFeeRate{
		SymbolID:            msg.GetSymbolId(),
		Symbol:              catalogSymbol(cats, msg.GetSymbolId()),
		MakerFeeRatePercent: msg.GetMakerFeeRatePercent(),
		TakerFeeRatePercent: msg.GetTakerFeeRatePercent(),
		VIPTier:             msg.GetVipTier(),
	}
}

// SpotFeeRatesListFromProto decodes GetSpotFeeRatesResponse.
func SpotFeeRatesListFromProto(msg *feesv1.GetSpotFeeRatesResponse, cats *catalogs.Manager) models.SpotFeeRatesList {
	rows := msg.GetFeeRates()
	out := make([]models.SpotFeeRate, 0, len(rows))
	for _, item := range rows {
		if item == nil {
			continue
		}
		out = append(out, SpotFeeRateFromProto(item, cats))
	}
	return models.SpotFeeRatesList{FeeRates: out}
}
