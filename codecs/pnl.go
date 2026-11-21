package codecs

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	pnlv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/pnl/v1"
)

var profitAndLossWindowToProto = map[string]pnlv1.ProfitAndLossWindow{
	"today":    pnlv1.ProfitAndLossWindow_PROFIT_AND_LOSS_WINDOW_TODAY,
	"days_7":   pnlv1.ProfitAndLossWindow_PROFIT_AND_LOSS_WINDOW_DAYS_7,
	"days_30":  pnlv1.ProfitAndLossWindow_PROFIT_AND_LOSS_WINDOW_DAYS_30,
	"lifetime": pnlv1.ProfitAndLossWindow_PROFIT_AND_LOSS_WINDOW_LIFETIME,
}

// ResolveProfitAndLossWindow maps a window label (today, days_7, days_30 or
// lifetime; case is ignored) to its contract value. The window is required.
func ResolveProfitAndLossWindow(label string) (pnlv1.ProfitAndLossWindow, error) {
	if value, ok := profitAndLossWindowToProto[strings.ToLower(strings.TrimSpace(label))]; ok {
		return value, nil
	}
	return pnlv1.ProfitAndLossWindow_PROFIT_AND_LOSS_WINDOW_UNSPECIFIED, &errors.ValidationError{Msg: "window must be one of 'today', 'days_7', 'days_30', or 'lifetime'"}
}

// ResolveProfitAndLossProduct maps a product label to its contract value:
// "" for every product, or "spot".
func ResolveProfitAndLossProduct(label string) (pnlv1.ProfitAndLossProduct, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "":
		return pnlv1.ProfitAndLossProduct_PROFIT_AND_LOSS_PRODUCT_UNSPECIFIED, nil
	case "spot":
		return pnlv1.ProfitAndLossProduct_PROFIT_AND_LOSS_PRODUCT_SPOT, nil
	}
	return pnlv1.ProfitAndLossProduct_PROFIT_AND_LOSS_PRODUCT_UNSPECIFIED, &errors.ValidationError{Msg: "product must be empty (every product) or 'spot'"}
}
