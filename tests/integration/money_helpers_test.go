//go:build integration

package integration_test

import (
	"fmt"
	"math/big"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func pricePtr(p models.PriceInput) *models.PriceInput { return &p }

// feeAmountE18ToAssetScaled converts a fee_amount_e18 decimal string into the
// fee asset's quantity scale. Fees are exact at ledger scale 18.
func feeAmountE18ToAssetScaled(feeE18 string, assetScale int) (int64, error) {
	if feeE18 == "" || feeE18 == "0" {
		return 0, nil
	}
	value, ok := new(big.Int).SetString(feeE18, 10)
	if !ok {
		return 0, fmt.Errorf("invalid fee_amount_e18 %q", feeE18)
	}
	if assetScale < 0 || assetScale > codecs.LedgerScale {
		return 0, fmt.Errorf("invalid asset scale %d", assetScale)
	}
	diff := codecs.LedgerScale - assetScale
	if diff == 0 {
		if !value.IsInt64() {
			return 0, fmt.Errorf("fee_amount_e18 %q overflows int64", feeE18)
		}
		return value.Int64(), nil
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(diff)), nil)
	quot, rem := new(big.Int).QuoRem(value, divisor, new(big.Int))
	if rem.Sign() != 0 {
		return 0, fmt.Errorf("fee_amount_e18 %q not exact at scale %d", feeE18, assetScale)
	}
	if !quot.IsInt64() {
		return 0, fmt.Errorf("fee at asset scale overflows int64")
	}
	return quot.Int64(), nil
}
