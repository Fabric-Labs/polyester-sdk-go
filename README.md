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
./scripts/setup-hooks.sh   # optional: lint before each git commit
```

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
