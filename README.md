# Polyester Go SDK

Go SDK for the Polyester public API — parity with `polyester-sdk-python` using the checked-in `gen/` protobuf bundle (no local proto generation required for normal development).

## Install

```bash
go get github.com/Fabric-Labs/polyester-sdk-go@latest
```

## Quick start

```go
package main

import (
    "context"
    "fmt"
    "log"

    polyester "github.com/Fabric-Labs/polyester-sdk-go"
    "github.com/Fabric-Labs/polyester-sdk-go/services"
)

func main() {
    client, err := polyester.FromEnv()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx := context.Background()
    me, err := client.Auth.Me(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("account:", me.AccountID)

    spot, err := client.MarketData.GetSpotConfig(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("pairs:", len(spot.Raw))
}
```

Set `POLYESTER_API_KEY_ID`, `POLYESTER_API_PRIVATE_KEY`, and optionally `POLYESTER_ACCOUNT_ID`.

`FromEnv` starts best-effort spot and Zipper catalog hydration in the background.
Wait for it before decimal writes that depend on catalog scales:

```go
if err := client.WaitForCatalogs(ctx); err != nil {
    log.Fatal(err)
}
```

`WaitForCatalogs` returns immediately when `HydrateCatalogs` is disabled.

## Qty / price inputs (dual path)

Human path uses decimal strings. Bot path stays in wire units with typed values.
`PriceTicks.Ticks` are protocol price units (fixed 1e6), not market tick-size alignment
(the server still validates tick size).

```go
import "github.com/Fabric-Labs/polyester-sdk-go/models"

symbol := "BNB-USDT"
tif := "gtc"
price := models.PriceFromDecimal("100")
_, err = client.Orders.Create(ctx, models.CreateOrderRequest{
    Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif,
    Qty: models.QtyFromDecimal("0.01"), Price: &price, PostOnly: true,
}, nil)
```

### For bots (scaled integers)

```go
qty := models.MustQtyScaled(1_000_000).WithScale(8) // already wire units
price := models.PriceFromTicksInt(100_000_000)        // 100.000000 at 1e6
_, err = client.Orders.Create(ctx, models.CreateOrderRequest{
    Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif,
    Qty: models.QtyFromScaled(qty), Price: &price, PostOnly: true,
}, nil)
// Reads expose the same types: order.Price.Ticks, order.OrigQty.Scaled
```

Compatible fill/book values can be passed back into writes when domain/instrument match.
Transfers and trading withdraws use `AssetAmountInput` (not order `QtyInput`).

## Realtime

Raw Centrifugo subscriptions return a typed channel:

```go
sub, err := client.Orders.Subscribe(ctx, nil)
if err != nil { log.Fatal(err) }
defer sub.Close()

for order := range sub.Messages() {
    fmt.Println(order.Symbol)
}
```

Managed snapshot-then-stream helpers:

```go
ob, err := client.Orderbook.CreateSubscription(ctx, services.CreateSubscriptionOptions{
    Symbol: "ETH-USDT",
    Depth:  50,
})
if err != nil { log.Fatal(err) }
defer ob.Close()

for book := range ob.Updates() {
    fmt.Println(book.BookSeq, len(book.Bids))
}
```

## Development

```bash
make test              # unit tests only (no network)
make test-integration  # live devnet smoke tests (reads .env)
make test-all          # unit + integration (sources .env automatically)
make lint              # golangci-lint (moon run :lint when moon is installed)
make fix               # golangci-lint --fix (moon run :fix when moon is installed)
make coverage          # regenerate Connect RPC ↔ wrapper coverage reports
./scripts/setup-hooks.sh   # optional: lint before each git commit
```

Connect RPC wrapper coverage (public gen vs handwritten services) is tracked in
[`docs/sdk-coverage.md`](docs/sdk-coverage.md). CI fails on unexpected gaps or a
stale report — after wrapping a new RPC or refreshing gen, run `make coverage`.

Unit tests run offline. Integration tests use `//go:build integration` and need API keys in `.env` (same variables as Python: `POLYESTER_API_KEY_ID`, `POLYESTER_API_PRIVATE_KEY`, optional `POLYESTER_ACCOUNT_ID`, `POLYESTER_API_URL`).

`make test-integration` and `testutil.LiveClientFromEnv` load `.env` from the repo root when present. You can also export vars manually:

```bash
set -a && source .env && set +a
go test -tags=integration -v ./tests/integration/...
```

Integration tests skip automatically when API keys are not set.

## Layout

| Path | Purpose |
|------|---------|
| `client.go` | Root `Client`, `FromEnv`, service tree |
| `services/` | Typed unary RPC + subscribe helpers |
| `realtime/` | Centrifugo client, snapshot-then-stream |
| `orderbook/` | Local book merge + managed subscription |
| `codecs/` | Request encoding, scalar helpers, decode |
| `gen/` | Checked-in Connect + protobuf generated code |

## Note on protos

The Go SDK ships with a committed `gen/` tree. Proto stubs are updated when Yvan lands a new `gen/` bundle (same source as Python). You do not need local buf generation for day-to-day SDK work.
