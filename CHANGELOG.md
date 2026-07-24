# Changelog

## Unreleased

### Breaking
- Stable MFA auth error codes (POLY-2919): `AUTH_API_KEY_MFA_REQUIRED` is removed; use `AUTH_MFA_NOT_ENROLLED`, `AUTH_STEP_UP_REQUIRED`, `AUTH_MFA_ELEVATION_REQUIRED`, and `AUTH_MFA_LAST_FACTOR_REQUIRED` from `AuthErrorDetail`
- Generated `*.pb.validate.go` outputs are no longer shipped with the SDK bundle

### Features
- `errors.IsMFAEnrollmentRequired` / `IsStepUpRequired` / `IsMFAElevationRequired` / `IsMFALastFactorRequired` classify MFA control flow from structured auth codes only (no message heuristics)
- Public method options expose `polyester.api.MFARequirement` documentation metadata

### Testing
- Unit coverage for MFA auth-code mapping and predicates

### Changed
- CI no longer auto-commits `sdk-capabilities.json` / README on pull requests. Capability refresh + optional bot commit runs only on merge to `main`.

## 0.1.0a13

### Breaking
- POLY-3701 explicit execution variants: order/trigger create now encode onto typed execution oneofs. The flat public inputs (`order_type`/`tif`/`post_only`) are mapped to `OrderIntent` execution variants (`market`→`MarketIoc`, `limit`+`ioc`→`LimitIoc`, `limit`+`fok`→`LimitFok`, `limit`+`gtc`/default→`LimitGtc`); `post_only` is rejected for non-GTC executions
- Trigger create maps onto `TriggerIntent` strategy oneofs (`stop_loss`/`take_profit`→`ConditionalTrigger` + child, `trailing_stop`→implicit SELL market-IOC `TrailingStopTrigger`, `twap`→`TwapTrigger`, `ladder`→linear-only `LadderTrigger`); `trigger_price_source` and non-linear ladder distributions are no longer sent
- `orders.batch_create` dropped `allow_partial`; the wrapper argument is retained but ignored, and batch items now carry an Accepted/Rejected outcome
- `CreateOrderResponse` / `CreateTriggerResponse` no longer include a status field; the SDK synthesizes `"accepted"` for the mutation result
- Attached-risk take-profit/stop-loss policies now carry `trigger_price_ticks` + a child execution; `order_type` is derived from the child and `trigger_price_source` is no longer decoded
- `Trigger` read model now exposes full proto fields (order params, timestamps, detail blocks, `PostOnly`, `ParentOrderID`, child order ids), projected from the read `Configuration` + `RuntimeDetails` oneofs
- `Order` read model adds `PostOnly` and `AttachedRisk`
- `Triggers.List` gains a validated `status` filter argument (`created`/`armed`/`running`/`completed`/`cancelled`/`failed`/`paused`)

### Fixed
- `Orders.Get` / list with `includeAttachedRisk` now returns policy data on `Order.AttachedRisk`

## 0.1.0a12

### Fixed
- `CreateSubaccountResult.Revision` is returned from create so clients can pass `expected_revision` on the next mutation without a follow-up read

## 0.1.0a11

### Breaking
- Durable auth PATCH contract: subaccount, API key, subaccount/API policy, and address-book entry updates use nested mutable specs, a non-empty FieldMask, and a positive `expected_revision`
- Policy creates nest fields under `policy`; update APIs take presence-aware patch structs (pointers / `ClearableTime`) so `""`, empty slices, `false`, `0`, and null timestamp clears survive
- Soft-delete subaccount requires `expected_revision`; address-book entry updates drop `new_tags` (paths: `label`, `note`, `tag_ids`); tag updates use optional `*string` name/color without revision/mask
- Durable resource models expose `Revision`; Connect `AuthErrorDetail` maps `AUTH_REVISION_CONFLICT` onto `APIError.Code`

### Testing
- Live funded UserOp tests: Funding → Trading and Funding → external withdraw, gated by `POLYESTER_TEST_CHAIN_USEROP=1`
- Unit coverage for nested FieldMask builders, presence/clear semantics, revision decode, and revision-conflict error mapping

### Docs
- Realtime and on-chain Funding helpers remain required dependencies (already hard `require`s in `go.mod`); aligns with Python/Rust packaging.

## 0.1.0a10

### Features
- Optional `chain` package smart-account path: CREATE2 Safe prediction, ERC-4337 UserOp submit (bundler + paymaster), Funding → external / Funding → Trading calldata, Zipper fee quote, and full FundingAccount / GuardRegistry whitelist encoders
- Realtime delivery is fail-closed on queue overflow (`QueueOverflowError`); managed snapshot-then-stream subscriptions refresh Connect snapshots on reconnect and expose recovery callbacks
- Catalog hydration uses a mutex to avoid data races under concurrent readers

### Changed
- Connect RPC coverage gate no longer commits dashboard reports under `docs/`; CI fails on unexpected gaps only (`sdk-coverage.toml` + `scripts/check_sdk_coverage.py`)

### Docs
- README `Supported surface` table is generated from `sdk-capabilities.json` (`--write-capabilities`); links to the public [SDK capability matrix](https://polyester.ai/docs/developer-docs/getting-started/sdk-capability-matrix)
- README expanded toward Python parity (credentials, auth patterns, orders, balances, market data, realtime)
- CI auto-commits refreshed `sdk-capabilities.json` + README capability table when they drift (same-repo)
- README documents on-chain Funding UserOps (caller-supplied owner EOA → derive Polyester Safe) vs Trading withdraw RPCs; realtime overflow / reconnect recovery contract

### Testing
- Live smoke on Polyester testnet: Funding → BSC USDT withdraw UserOp via `chain.SmartAccount`
- Snapshot-then-stream unit coverage for `OnSnapshotRefresh`

## 0.1.0a9

### Features
- `Client.WaitForCatalogs(ctx)` waits for best-effort background catalog hydration (parity with Python/Rust)
- Connect RPC wrapper coverage gate: `scripts/check_sdk_coverage.py` + `sdk-coverage.toml`

### Docs
- README documents `WaitForCatalogs` before decimal writes that depend on catalog scales

## 0.1.0a8

### Breaking
- Authoritative freshness: `Order.StateRevision` → `Order.Version`; balance `TradingVersion` / `FundingVersion` / `ReservedVersion` → `TradingUpdatedAtNs` / `FundingUpdatedAtNs` / `ReservedUpdatedAtNs`; subaccount and API-key `UpdatedAt` are configuration timestamps; API-key `LastUsedAt` stays independent activity time

### Features
- Generated reconciliation and policy types exposed in the public SDK surface
- Internal transfer amounts use U128 wire types end-to-end

## 0.1.0a7

### Breaking
- Dual-path qty/price scalars: reads expose typed `Price` / `Quantity` / `AssetAmount` domain fields (`.Ticks` / `.Scaled`) instead of primary digit-string `PriceTicks` / `QtyScaled` fields
- Order, trigger, transfer, and withdraw writes accept human decimals or bot scaled inputs (`PriceInput` / `QtyInput` / `AssetAmountInput`); excess precision is rejected (no silent truncation)

### Money types
- New `models.Price`, `models.Quantity`, and `models.AssetAmount` helpers plus codec helpers for human and scaled/tick paths, with domain/scale compatibility checks on reuse

### Docs
- README documents human vs bot dual-path usage

### Testing
- Unit coverage for money encode/decode paths
- Integration asserts and helpers updated for typed price/qty reads

## 0.1.0a6

### Breaking
- Removed private `ledger.write.v1` / `Client.LedgerWrite` from the public SDK (not in `public.files.txt`)

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
