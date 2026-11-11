package hardening_test

// POLY-9228: what a failed catalog load reports, its deadline, and funding
// readiness on its own.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"google.golang.org/protobuf/proto"
)

const fundingOperation = "chain.zipper.v1.ZipperService/GetDepositWithdrawConfig"

func assertRedacted(t *testing.T, err error) {
	t.Helper()
	for e := err; e != nil; e = errors.Unwrap(e) {
		text := e.Error() + fmt.Sprintf(" %+v", e)
		if strings.Contains(text, startupProxySecret) || strings.Contains(text, startupProxyUser) {
			t.Fatalf("error leaks the proxy credentials: %s", text)
		}
	}
}

func waitFunding(t *testing.T, client *polyester.Client) *sdkerrors.CatalogHydrationError {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := client.WaitForFundingCatalog(ctx)
	var hydrationErr *sdkerrors.CatalogHydrationError
	if !errors.As(err, &hydrationErr) {
		t.Fatalf("WaitForFundingCatalog = %v, want *CatalogHydrationError", err)
	}
	return hydrationErr
}

func TestL2PersistentFundingStallFailsWithinDeadline(t *testing.T) {
	h := newStartupHarness(t, stallAlways("GetDepositWithdrawConfig"))
	cfg := h.config(500 * time.Millisecond)
	cfg.CatalogTimeout = 1500 * time.Millisecond
	started := time.Now()
	client := h.client(t, cfg)
	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
	failure := waitFunding(t, client)
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("the stall surfaced after %s", elapsed)
	}
	if failure.Catalog != "funding" || failure.Operation != fundingOperation || failure.Failure != "timeout" {
		t.Fatalf("failure = %+v", failure)
	}
	if failure.Attempts < 2 || failure.Attempts != h.api.count("GetDepositWithdrawConfig") {
		t.Fatalf("attempts = %d, reads = %d", failure.Attempts, h.api.count("GetDepositWithdrawConfig"))
	}
	// Each retry opened a tunnel of its own instead of queueing behind the stall.
	if tunnels := h.proxy.tunnels.Load(); tunnels < int64(1+failure.Attempts) {
		t.Fatalf("tunnels = %d for %d attempts", tunnels, failure.Attempts)
	}
	for _, want := range []string{"catalog hydration failed: funding config", fundingOperation, "timed out"} {
		if !strings.Contains(failure.Error(), want) {
			t.Fatalf("%q lacks %q", failure.Error(), want)
		}
	}
	assertRedacted(t, failure)
	if _, err := client.Zipper.SubscribeZippedAssetSupply(context.Background(), true); err == nil ||
		!strings.Contains(err.Error(), "funding config") {
		t.Fatalf("SubscribeZippedAssetSupply = %v, want the funding config's failure", err)
	}
	if client.Catalogs.LedgerIDForAsset("USDT") != nil {
		t.Fatal("funding lookup answered without the funding config")
	}
	// Spot trading still works once funding has failed.
	assertSpotOrderReachesServer(t, client, h.api)
}

func TestL2PersistentSpotStallFailsClosedWithinDeadline(t *testing.T) {
	h := newStartupHarness(t, stallAlways("GetSpotConfig"))
	cfg := h.config(500 * time.Millisecond)
	cfg.CatalogTimeout = 1500 * time.Millisecond
	started := time.Now()
	client := h.client(t, cfg)
	err := client.WaitForCatalogs(context.Background())
	var failure *sdkerrors.CatalogHydrationError
	if !errors.As(err, &failure) || failure.Catalog != "spot" || failure.Failure != "timeout" {
		t.Fatalf("WaitForCatalogs = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("the stall surfaced after %s", elapsed)
	}
	assertRedacted(t, err)
	_, err = client.Orders.Create(context.Background(), limitOrder(startupSymbol), nil)
	if !errors.As(err, &failure) || !strings.Contains(err.Error(), "spot config") {
		t.Fatalf("Orders.Create = %v, want the spot config's failure", err)
	}
	if h.api.count("CreateOrder") != 0 {
		t.Fatal("an order was sent without the spot config")
	}
	// The funding config loaded on its own.
	if err := client.WaitForFundingCatalog(context.Background()); err != nil {
		t.Fatalf("WaitForFundingCatalog: %v", err)
	}
}

func TestL2PersistentlyDroppedFundingReadNamesAClosedConnection(t *testing.T) {
	h := newStartupHarness(t, func(operation string, _ int) startupAction {
		if operation == "GetDepositWithdrawConfig" {
			return startupDrop
		}
		return startupOK
	})
	client := h.client(t, h.config(5*time.Second))
	failure := waitFunding(t, client)
	if failure.Failure != "connection_closed" || failure.Attempts != 3 || h.api.count("GetDepositWithdrawConfig") != 3 {
		t.Fatalf("failure = %+v after %d reads", failure, h.api.count("GetDepositWithdrawConfig"))
	}
	if !strings.Contains(failure.Error(), "connection closed after 3 attempts") {
		t.Fatalf("message = %q", failure.Error())
	}
	var serverErr *sdkerrors.ServerError
	if !errors.As(failure, &serverErr) {
		t.Fatalf("cause = %T, want *ServerError", failure.Err)
	}
	assertRedacted(t, failure)
}

func TestL2UnavailableFundingReadStopsAfterThreeAttemptsWithItsReference(t *testing.T) {
	var reads atomic.Int64
	srv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		switch {
		case strings.HasSuffix(req.Path, "/GetSpotConfig"):
			return startupSpotResponse(t)
		case strings.HasSuffix(req.Path, "/GetDepositWithdrawConfig"):
			n := reads.Add(1)
			return hardening.Raw(http.StatusServiceUnavailable, [][2]string{
				{"Content-Type", "application/json"},
				{"Cf-Ray", fmt.Sprintf("a5711ea9d826%04d-LAX", n)},
			}, []byte(`{"code":"unavailable","message":"edge unavailable"}`))
		}
		return hardening.NotFound()
	})
	t.Cleanup(srv.Close)
	client, err := polyester.New(polyester.Config{
		APIURL: srv.BaseURL(), WSURL: "ws://127.0.0.1:1/unused", HydrateCatalogs: true, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
	failure := waitFunding(t, client)
	if reads.Load() != 3 || failure.Attempts != 3 || failure.Failure != "unavailable" {
		t.Fatalf("failure = %+v after %d reads", failure, reads.Load())
	}
	if failure.ResponseRef != "a5711ea9d8260003-LAX" || !strings.Contains(failure.Error(), "response ref a5711ea9d8260003-LAX") {
		t.Fatalf("response ref = %q in %q", failure.ResponseRef, failure.Error())
	}
}

func TestL2InvalidSpotConfigIsNotRetriedAndFailsClosed(t *testing.T) {
	var reads atomic.Int64
	srv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		switch {
		case strings.HasSuffix(req.Path, "/GetSpotConfig"):
			reads.Add(1)
			return hardening.Raw(http.StatusOK, [][2]string{{"Content-Type", "application/proto"}}, nil)
		case strings.HasSuffix(req.Path, "/GetDepositWithdrawConfig"):
			return hardening.Raw(http.StatusOK, [][2]string{{"Content-Type", "application/proto"}}, nil)
		}
		return hardening.NotFound()
	})
	t.Cleanup(srv.Close)
	client, err := polyester.New(polyester.Config{
		APIURL: srv.BaseURL(), WSURL: "ws://127.0.0.1:1/unused", HydrateCatalogs: true, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	err = client.WaitForCatalogs(context.Background())
	var failure *sdkerrors.CatalogHydrationError
	if !errors.As(err, &failure) || failure.Failure != "invalid" || failure.Attempts != 1 || reads.Load() != 1 {
		t.Fatalf("WaitForCatalogs = %v (%+v) after %d reads", err, failure, reads.Load())
	}
	if !errors.Is(client.CatalogsLastError(), failure.Err) {
		t.Fatal("CatalogsLastError lost the failure")
	}
}

func TestCatalogTimeoutMustNotBeNegative(t *testing.T) {
	_, err := polyester.New(polyester.Config{CatalogTimeout: -time.Second})
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "CatalogTimeout") {
		t.Fatalf("New = %v", err)
	}
}

func startupSpotResponse(t *testing.T) hardening.HttpResponse {
	t.Helper()
	api := &startupAPI{plan: func(string, int) startupAction { return startupOK }, calls: map[string]int{}}
	resp, err := api.GetSpotConfig(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := proto.Marshal(resp.Msg)
	if err != nil {
		t.Fatal(err)
	}
	return hardening.Raw(http.StatusOK, [][2]string{{"Content-Type", "application/proto"}}, body)
}
