package services

import (
	"context"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// awaitCatalogs waits for the catalog load the client began when it was
// created, before a call resolves a display symbol or reads a pair's scales,
// so a call made right after New does not refuse a valid symbol. It returns
// at once when no load was begun (Config.HydrateCatalogs off), and ctx's
// error when ctx ends first; the load itself carries on.
//
// With required, a failed load returns *errors.CatalogHydrationError wrapping
// the failure. Without it, a failed load returns nil: the call only labels or
// scales its results from the catalogs and works without them.
func awaitCatalogs(ctx context.Context, cats *catalogs.Manager, required bool) error {
	if cats == nil {
		return nil
	}
	done := cats.Hydration()
	if done == nil {
		return nil
	}
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := cats.HydrationError(); err != nil && required {
		return &errors.CatalogHydrationError{Err: err}
	}
	return nil
}

// hydrationFailure wraps the failure of construction-time catalog hydration
// for a call that needs the catalogs.
func hydrationFailure(err error) error {
	if err == nil {
		return nil
	}
	return &errors.CatalogHydrationError{Err: err}
}

// namesSymbol reports whether a call names a market by display symbol, which
// it resolves through the catalogs.
func namesSymbol(symbol *string, symbols ...string) bool {
	if symbol != nil && strings.TrimSpace(*symbol) != "" {
		return true
	}
	for _, item := range symbols {
		if strings.TrimSpace(item) != "" {
			return true
		}
	}
	return false
}
