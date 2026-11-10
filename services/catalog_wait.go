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
	return awaitLoad(ctx, cats.Hydration(), cats.HydrationError, required)
}

// awaitFundingCatalog waits for the funding (deposit/withdraw) config the
// client began loading when it was created, as awaitCatalogs does for the
// spot config. A failed load returns *errors.CatalogHydrationError naming it.
func awaitFundingCatalog(ctx context.Context, cats *catalogs.Manager) error {
	if cats == nil {
		return nil
	}
	return awaitLoad(ctx, cats.FundingHydration(), cats.FundingHydrationError, true)
}

func awaitLoad(ctx context.Context, done <-chan struct{}, loadErr func() error, required bool) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := loadErr(); err != nil && required {
		return hydrationFailure(err)
	}
	return nil
}

// hydrationFailure wraps the failure of construction-time catalog hydration
// for a call that needs the catalogs. A failure that already is a
// *errors.CatalogHydrationError, as the client's own load returns, is
// returned as is, so errors.Is matches it with Client.CatalogsLastError.
func hydrationFailure(err error) error {
	if err == nil {
		return nil
	}
	if hydrationErr, ok := err.(*errors.CatalogHydrationError); ok {
		return hydrationErr
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
