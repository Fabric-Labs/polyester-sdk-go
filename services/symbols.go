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

const maxCancelAllSymbolIDs = 100

// ResolveCancelAllSymbolIDs resolves cancel-all Connect symbol_ids.
// Empty means all symbols. Duplicates are ignored. At most 100 positive IDs
// are accepted. symbol, symbols, and symbolIDs are mutually exclusive once
// any of them selects at least one pair.
func ResolveCancelAllSymbolIDs(catalogs *catalogs.Manager, symbol *string, symbols []string, symbolIDs []uint32, label string) ([]uint32, error) {
	normalizedSymbol := ""
	if symbol != nil {
		normalizedSymbol = strings.TrimSpace(*symbol)
	}
	normalizedSymbols := make([]string, 0, len(symbols))
	for _, item := range symbols {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		normalizedSymbols = append(normalizedSymbols, trimmed)
	}

	selected := 0
	if normalizedSymbol != "" {
		selected++
	}
	if len(normalizedSymbols) > 0 {
		selected++
	}
	if len(symbolIDs) > 0 {
		selected++
	}
	if selected > 1 {
		return nil, &errors.ValidationError{Msg: label + " accepts only one of symbol, symbols, or symbol_ids"}
	}

	resolved := make([]uint32, 0, len(symbolIDs)+len(normalizedSymbols)+1)
	switch {
	case len(symbolIDs) > 0:
		for _, id := range symbolIDs {
			if id == 0 {
				return nil, &errors.ValidationError{Msg: label + " symbol_ids must be positive"}
			}
			resolved = append(resolved, id)
		}
	case normalizedSymbol != "":
		id, err := ResolveSymbolID(catalogs, &normalizedSymbol, nil, label+" symbol")
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, id)
	default:
		for _, item := range normalizedSymbols {
			id, err := ResolveSymbolID(catalogs, &item, nil, label+" symbols")
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, id)
		}
	}

	unique := make([]uint32, 0, len(resolved))
	seen := make(map[uint32]struct{}, len(resolved))
	for _, id := range resolved {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) > maxCancelAllSymbolIDs {
		return nil, &errors.ValidationError{Msg: label + " accepts at most 100 symbol_ids"}
	}
	return unique, nil
}
