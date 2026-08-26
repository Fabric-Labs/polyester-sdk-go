package services

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// ResolveSymbolID resolves a required symbol_id from an explicit id or catalog
// lookup of a display symbol. Connect is symbol_id-only except GetSpotConfig;
// never forward a raw display symbol to the API.
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

// ResolveOptionalSymbolID resolves an optional symbol filter.
// Empty or omitted display symbol stays 0 (all symbols). A supplied display
// symbol is resolved through the catalog and fails closed when unknown.
func ResolveOptionalSymbolID(catalogs *catalogs.Manager, symbol *string, symbolID *uint32, label string) (uint32, error) {
	if symbolID != nil {
		return *symbolID, nil
	}
	if symbol == nil || strings.TrimSpace(*symbol) == "" {
		return 0, nil
	}
	trimmed := strings.TrimSpace(*symbol)
	return ResolveSymbolID(catalogs, &trimmed, nil, label)
}
