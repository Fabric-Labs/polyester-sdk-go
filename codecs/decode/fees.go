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
		SymbolID:                      msg.GetSymbolId(),
		Symbol:                        catalogSymbol(cats, msg.GetSymbolId()),
		MakerFeeRatePercent:           msg.GetMakerFeeRatePercent(),
		TakerFeeRatePercent:           msg.GetTakerFeeRatePercent(),
		VIPTier:                       msg.GetVipTier(),
		ReserveFeeRatePercent:         msg.GetReserveFeeRatePercent(),
		PostOnlyReserveFeeRatePercent: msg.GetPostOnlyReserveFeeRatePercent(),
		NextChangeAt:                  timestampTime(msg.GetNextChangeAt()),
		Breakdown:                     spotFeeBreakdown(msg.GetBreakdown()),
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

func spotFeeBreakdown(msg *feesv1.SpotFeeRateBreakdown) *models.SpotFeeRateBreakdown {
	if msg == nil {
		return nil
	}
	discounts := make([]models.SpotFeeDiscount, 0, len(msg.GetAccountDiscounts()))
	for _, item := range msg.GetAccountDiscounts() {
		if item != nil {
			discounts = append(discounts, *spotFeeDiscount(item))
		}
	}
	return &models.SpotFeeRateBreakdown{
		TierMakerFeeRatePercent: msg.GetTierMakerFeeRatePercent(),
		TierTakerFeeRatePercent: msg.GetTierTakerFeeRatePercent(),
		MarketMultiplier:        spotFeeMultiplier(msg.GetMarketMultiplier()),
		AccountDiscounts:        discounts,
		ReferralDiscount:        spotFeeDiscount(msg.GetReferralDiscount()),
	}
}
func spotFeeMultiplier(msg *feesv1.SpotFeeMultiplier) *models.SpotFeeMultiplier {
	if msg == nil {
		return nil
	}
	return &models.SpotFeeMultiplier{
		MakerMultiplierPercent: msg.GetMakerMultiplierPercent(),
		TakerMultiplierPercent: msg.GetTakerMultiplierPercent(),
		EffectiveFrom:          timestampTime(msg.GetEffectiveFrom()),
		EffectiveTo:            timestampTime(msg.GetEffectiveTo()),
	}
}
func spotFeeDiscount(msg *feesv1.SpotFeeDiscount) *models.SpotFeeDiscount {
	if msg == nil {
		return nil
	}
	return &models.SpotFeeDiscount{
		MakerDiscountPercent: msg.GetMakerDiscountPercent(),
		TakerDiscountPercent: msg.GetTakerDiscountPercent(),
		AllMarkets:           msg.GetAllMarkets(),
		EffectiveFrom:        timestampTime(msg.GetEffectiveFrom()),
		EffectiveTo:          timestampTime(msg.GetEffectiveTo()),
	}
}
