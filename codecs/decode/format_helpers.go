package decode

import "github.com/Fabric-Labs/polyester-sdk-go/codecs"

func formatQtyScaledOrEmpty(qtyScaled int64, scale int) string {
	out, err := codecs.FormatQtyScaled(qtyScaled, scale)
	if err != nil {
		return ""
	}
	return out
}

func formatLedgerU128OrZero(raw string, scale int) string {
	out, err := codecs.FormatLedgerU128(raw, scale)
	if err != nil {
		return "0"
	}
	return out
}
