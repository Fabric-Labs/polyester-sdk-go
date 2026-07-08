# Changelog

## Unreleased

## 0.1.0a5

### Triggers
- `Triggers.Create` accepts a `CreateTriggerOptions` struct exposing the full `CreateTriggerRequest` surface, unblocking `TRAILING_STOP`, `TWAP`, and `LADDER` creation: fee source, self-trade prevention mode, trailing distance (ticks/bps), activation price, max slippage (ticks/bps), TWAP window, and ladder range/levels/distribution
- `Triggers.Create` now takes `triggerPrice` as an optional `*string` (not required for trailing/TWAP/ladder types)

### Testing
- Trigger creation integration test updated for the new `Create` signature

## 0.1.0a4

### Breaking
- `Orders.CancelAll` no longer accepts `maxOrders`; the field was removed from the public proto and server no longer honors it

### Orders
- Market orders: `CreateOrderRequest.MarketClientRefPrice` for IOC market buys when the orderbook may be empty
- Proto gen bump (2026-07-03 bundle) aligned with server contract

### Testing
- Market order mutation integration tests (single-account IOC buy/sell on devnet)
- Market order fill and spot fill e2e with optional maker credentials (`POLYESTER_TEST_MAKER_*`)
- `TestBalancesGetHealth` skips when route is not mounted

### CI
- golangci-lint v2.6.2 (Go 1.25 support) and v2 config schema (`linters.exclusions.paths`)
