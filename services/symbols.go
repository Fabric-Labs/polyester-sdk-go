package services

import (
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// ResolveSymbolID resolves symbol_id from explicit id or catalog lookup.
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

// ValidateSymbolFilter verifies a raw symbol filter against the hydrated spot
// catalog. nil and empty filters preserve the endpoint's unfiltered semantics.
func ValidateSymbolFilter(catalogs *catalogs.Manager, symbol *string, label string) error {
	if symbol == nil || *symbol == "" {
		return nil
	}
	_, err := ResolveSymbolID(catalogs, symbol, nil, label)
	return err
}

// ValidateSymbolFilters verifies every non-empty raw symbol filter.
func ValidateSymbolFilters(catalogs *catalogs.Manager, symbols []string, label string) error {
	for i := range symbols {
		if symbols[i] == "" {
			continue
		}
		if err := ValidateSymbolFilter(catalogs, &symbols[i], label); err != nil {
			return err
		}
	}
	return nil
}
