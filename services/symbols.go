package services

import (
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// ResolveSymbolID resolves symbol_id from explicit id or catalog lookup.
// Use this only for endpoints that send wire symbol_id; raw-symbol filters are
// forwarded to the API without catalog fail-closed checks.
func ResolveSymbolID(catalogs *catalogs.Manager, symbol *string, symbolID *uint32, label string) (uint32, error) {
	if symbolID != nil {
		return *symbolID, nil
	}
	if symbol != nil && catalogs != nil {
		if resolved := catalogs.SymbolIDForSymbol(*symbol); resolved != nil {
			return *resolved, nil
		}
	}
	if symbol == nil {
		return 0, &errors.ValidationError{Msg: label + " requires symbol or symbol_id"}
	}
	return 0, &errors.ValidationError{Msg: "Unknown symbol '" + *symbol + "'; call get_spot_config first or pass symbol_id"}
}
