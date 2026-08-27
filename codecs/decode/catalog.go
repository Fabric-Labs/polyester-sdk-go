package decode

import "github.com/Fabric-Labs/polyester-sdk-go/catalogs"

func catalogSymbol(cats *catalogs.Manager, symbolID uint32) string {
	if cats == nil || symbolID == 0 {
		return ""
	}
	if symbol := cats.SymbolForSymbolID(symbolID); symbol != nil {
		return *symbol
	}
	return ""
}
