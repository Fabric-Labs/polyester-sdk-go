# Changelog

## Unreleased

## 0.1.0a39

Git tag: `v0.1.0a39`.

### Breaking
- `Orders.ListOpen` and `Orders.ListHistory` gain a trailing `triggerID *string`
  filter. When set, only child orders created by that trigger are returned
  (useful for TWAP/ladder slice children and their execution prices).
- Generated `TriggerEvent.fire_price_ticks` is now optional (absent for
  time-scheduled TWAP slice fires). Decoded `TriggerEvent.FirePrice` stays
  empty when the wire field is unset.

## 0.1.0a38

Git tag: `v0.1.0a38`.

### Breaking
- `Lifecycle.GetFlowByTx` now returns `models.LifecycleFlowsList` instead of
  silently returning only the first match. A non-positive limit now defaults
  to 50; use `Lifecycle.ListFlowsByTx` with `NextPageToken` for pagination.

### Added
- Generated `polyester.ratelimit.v1` contracts expose structured quota
  rejection details used by order and trade WebSocket responses.

## 0.1.0a37

Git tag: `v0.1.0a37`.

### Added
- `Withdraw.ValidateDestination` wraps `ValidateWithdrawDestination` and maps
  validation codes to snake labels (`valid`, `invalid_address`,
  `denylisted_address`, …) for preflight checks before Trading → external
  withdraws.

## 0.1.0a36

Git tag: `v0.1.0a36`.

### Added
- Lifecycle reason catalog maps trading-withdraw failure codes to snake labels:
  `trading_withdraw_policy_denied`, `trading_withdraw_contract_reverted`, and
  `trading_withdraw_execution_failed` .

## 0.1.0a35

Git tag: `v0.1.0a35`.

### Docs
- README documents `UserTrade` fee e18 fields and `FeeIsRebate` polarity.
- README / capabilities note: order create/modify do not accept `AttachedRisk`
  (decode-only; use Rust/Python for attached create/modify).

## 0.1.0a34

Git tag: `v0.1.0a34`.

### Breaking
- `UserTrade` fee fields move from asset-scaled integers to fixed 18-decimal
  magnitudes: `FeeScaled` → `FeeAmountE18`, `ReferralShareScaled` →
  `ReferralShareAmountE18` (decimal strings of wire `U128`). Convert to the fee
  asset's catalog scale before subtracting from BUY fill quantity.
- `UserTrade` adds sparse `FeeIsRebate`. When true, `FeeAmountE18` is a rebate
  credit rather than a fee debit (proto3 omits false).

## 0.1.0a33

Git tag: `v0.1.0a33`.

### Fixed
- `Ed25519Keypair` `String` / `GoString` redact secret material (same posture as `Config`).
- Decode omits attached trailing legs that lack a positive distance (no fabricated
  zero-distance stop).

## 0.1.0a32

Git tag: `v0.1.0a32`.

### Breaking
- Trigger snapshots no longer expose `ChildOrderIDs`. Child-order history is
  authoritative on trigger events: use `Triggers.ListEvents(..., eventType, ...)`
  with `eventType="fired"` and read `ChildOrderID` / `ChildSeq`.
- `Triggers.ListEvents` gains an `eventType *string` argument before
  `pageToken`.
- Decoded `TriggerEvent.EventType` labels are now `fired` / `canceled` /
  `updated` (not proto names like `EVENT_FIRED`).

### Added
- `TriggerEvent` thickens with `SubaccountID`, `SymbolID`, `TriggerType`,
  `ChildSeq`, `ChildOrderID`, `FirePrice`, and `Reason`.

## 0.1.0a31

Git tag: `v0.1.0a31`.

### Breaking
- `PreviewOrderResult` no longer returns fee/quote estimates
  (`EstimatedQuoteDebit`, `EstimatedFee`, `EstimatedNetBaseQty`, `FeeAsset`,
  `FreshAtTsNs`) or `PriceBound`. It now exposes admission fields:
  optional `Admissible`, optional `Rejection` (`OrderErrorDetail` with
  `OrderFieldViolation`s), optional resolved base quantity,
  `ProtectedPriceBound` (renamed from `PriceBound`), and `EvaluatedAtMs`.
  Known Preview rejection codes use TypeScript-compatible labels such as
  `BAD_QTY`; unknown open-enum values use `UNKNOWN_ERROR_CODE(<n>)`.
- Lifecycle flow summaries map `lifecycle_reason` (formerly `reason_code` /
  `FlowReason`) to snake labels on `LifecycleFlowSummary.LifecycleReason`.
  Unknown reason codes decode as `unknown_reason_<n>` instead of failing the
  flow. Tx-match flows preserve `OwnerAccountID`.

### Added
- `OrderErrorDetail` / `OrderFieldViolation` for PreviewOrder rejection detail.
- `ZipperReasonDetails` and `LifecycleFlowSummary.ZipperReason` for Zipper
  failure codes, reason ids, and messages from `chain.zipper.v1`.

## 0.1.0a30

Git tag: `v0.1.0a30`.

### Breaking
- `CreateOrderRequest.MaxQuoteDebitScaled` is now a typed `QtyInput` with
  `QuantityDomainOrderQuote` instead of `*int64`. Construct quote budgets with
  `QtyFromQuoteDecimal` / `QtyFromQuoteScaled`; the SDK validates the embedded
  scale against the pair's catalog `quote_quantity_scale`.
- `PreviewOrderResult` now exposes typed `EstimatedQuoteDebit` and
  `EstimatedFee` (`QtyScaled`) instead of bare `*_Scaled` strings.
- Wire `PreviewOrderRequest` is no longer a flat sizing/execution message; it
  matches create: optional `subaccount_id` plus `order` (`OrderIntent`). Public
  `Orders.Preview` still accepts `CreateOrderRequest`.
- Wire `TrailingStopTrigger` carries `side` (`orders.v1.Side`). Standalone
  create still validates sell-only; read models project the wire side (buy or
  sell) instead of hardcoding sell.

### Added
- Catalog quote-quantity-scale lookup:
  `QuoteQuantityScaleForSymbol` / `QuoteQuantityScaleForSymbolID`.
- Local validation rejects create, cancel, and replace batches above 20 items.

### Fixed
- Transfer and trading-withdraw amounts fail closed when neither the
  `AssetAmount` nor request parameters provide a source scale.
- Spot-config decoding preserves valid zero quote quantity scales.
- Trailing-stop decode projects configuration `side` / type correctly and keeps
  `parent_order_id` for attached risk triggers.

## 0.1.0a29

Git tag: `v0.1.0a29`.

### Changed
- README install pin catches up to the current alpha tag. No API changes from `0.1.0a28`.

## 0.1.0a28

Git tag: `v0.1.0a28`.

### Breaking
- Order and trigger fee outputs now use `FeeAsset` (`quote` / `base`); the removed `received` value maps to `base`.

### Added
- `CreateOrderRequest` supports exactly one of base `Qty` or BUY quote-budget `MaxQuoteDebitScaled`; create results expose resolved base quantity and submitted quote debit.
- `Orders.Preview` wraps `PreviewOrder` and returns advisory resolved size, price bound, debit, and fee estimates.
- `models.BatchReplaceStatusSettled` / `models.IsBatchReplaceSettled` identify stable post-admission batch-replace phases.

### Fixed
- `GetBatchReplaceStatus` fails closed with `ResponseContractError` when accepted/rejected counters disagree with per-item phases.

## 0.1.0a27

Git tag: `v0.1.0a27`.

### Breaking
- `Orders.BatchModify` / `models.BatchModify*` are replaced by admission-oriented `Orders.BatchReplace` / `models.BatchReplace*` (`BatchReplaceItem`, `BatchReplaceOrdersResult`). The write RPC returns a durable admission receipt (`batch_request_id`, accepted/rejected counts); poll `Orders.GetBatchReplaceStatus` for recoverable execution finality. Per-item `behavior` / request `behavior_default` / `allow_partial` are gone; batch replace is same-symbol quote refresh only.

## 0.1.0a26

Git tag: `v0.1.0a26`.

### Breaking
- Order identity for `Get` / `Cancel` / `Modify` / `WaitForOrderTradesComplete` and batch cancel/modify items is now a typed `models.OrderKey` (`OrderKeyByID` / `OrderKeyByClientID`) instead of two optional `*string` fields.

### Fixed
- Reject market creates that also supply a limit `price`.
- Decimal price parsing stays exact (no float intermediate).
- TWAP trigger projection coverage for proto decode paths.

## 0.1.0a25

Git tag: `v0.1.0a25`.

### Fixed
- Outbound HTTP/Connect/JSON-RPC requests send an explicit `User-Agent: polyester-sdk-go/<version>` instead of Go's default `Go-http-client/1.1`, so edge WAF rules that ban browser signatures (Cloudflare error 1010) do not block the SDK before authentication.
- Cloudflare error 1010 responses are mapped to `TransportError` with an explicit WAF message instead of being misclassified as auth failures.

### Breaking
- `MarketData.GetCurrentCandle` returns `(*models.Candle, error)`; `nil` means no candle rows (aligns with Rust `Option<Candle>`). Previously an empty result was a zero-valued `models.Candle`.
- `decode.OrderbookFromProto` and `orderbook.LevelsFromProtoLevels` now return errors so malformed snapshots cannot silently produce corrupt local books.
- `PriceTicks`, `QtyScaled`, and `AssetAmountScaled` fields are private; use constructors, immutable metadata builders, and `Ticks()` / `Scaled()` / `Scale()` getters.
- Mutation decoders that validate successful response contracts now return `(result, error)`.

### Fixed
- Managed orderbooks reject malformed levels and invalid sequence ranges atomically, keep the prior sequence/book, and request a snapshot refresh.
- Snapshot depth `1` and `1000` requests now use the matching protocol variants.
- Singular cancel and lookup by client-order-id validate the documented identifier constraints before contacting the transport.
- API-key withdrawals can be prepared, deterministically signed, persisted, restored, and submitted unchanged; precomputed signatures require an explicit deadline.
- Withdraw and internal-transfer amounts are exactly rescaled to canonical `amount_e18` without rounding.
- Mutation response contract violations are non-retryable outcome-unknown errors.
- Snapshot refreshes atomically drain pre-ready publications and sequence-gap refreshes are bounded, coalesced, and fail-closed.

## 0.1.0a24

### Breaking
- Public local-orderbook helpers now return errors for malformed data, and cancel-all response codecs return `(result, error)` so direct callers must propagate validation/transport failures.

### Fixed
- Local orderbook bucketing uses checked multiply/add, rounds asks up, and rejects negative prices/quantities instead of wrapping or emitting corrupted levels.
- `CancelAll` / `CancelAllAfter` response decoding rejects empty or unknown statuses (`submitted`/`dry_run`, `armed`/`disabled`) instead of returning success for ambiguous payloads. Cancel-all now also surfaces `failed_cancels`.
- Client order/trigger IDs and caller-supplied request IDs are trimmed and validated locally for documented length and ASCII-character constraints before a request is sent.

### Testing
- Decoder coverage asserts empty/unknown cancel-all and cancel-all-after statuses fail closed.
- Orderbook unit coverage asserts negative levels and quantity overflow are rejected.

## 0.1.0a23

### Fixed
- `FormatID(0)` now returns the canonical base58 zero (`"1"`) instead of aliasing id `1` as `"2"`; Go now preserves distinct zero/one round-trips and matches Python/TypeScript encoding.

### Clarified
- Document that `CreateOrderRequest.ClientOrderID` is API-optional; set a stable value when retrying after ambiguous mutation failures.

### Testing
- The 10,000-identical-request authentication regression also verifies that scheduler timers continue advancing during bounded signing backpressure.

## 0.1.0a22

### Fixed
- Batch-modify and batch-cancel decoding reconcile aggregate counts against per-item outcomes and reject unknown/ambiguous result states.
- Columnar candles reject misaligned OHLCV arrays instead of emitting empty fields.
- Deposit-address creation and singular lifecycle lookups reject missing required entities instead of returning placeholder models.
- User trades expose fee source and referral share, so received-asset fees can be distinguished from quote fees and BUY net quantity can be calculated correctly.
- Concurrent identical requests receive unique authentication timestamps per API key with a five-second future-skew ceiling and bounded backpressure.

### Testing
- Public-service Connect fault injection covers inconsistent batch counts and misaligned candle columns in addition to decoder-level boundary tests.
- The funded BUY-to-SELL acceptance test waits for complete fill projection and sells net received base quantity after received-asset fees.

## 0.1.0a21

### Breaking
- `models.BalanceHistorySeries.BalanceQ` is now `[]uint64`, matching the protobuf wire type and preserving values above `math.MaxInt64`.
- `models.BalanceHistorySeries.AccountCode` is now `int32`, preserving unknown negative protobuf enum values instead of wrapping them into large unsigned values.
- Trading withdrawals require explicit non-empty idempotency key and non-zero nonce values.
- `auth.SignRequest` and canonical URL helpers now return errors for malformed absolute URLs.
- Scale-dependent market data, orderbook, and Zipper supply paths require hydrated catalog scales instead of guessing.

### Features
- Realtime subscriptions expose `SetOnError`; managed snapshot streams accept `OnError`.
- Retry classifiers and cryptographically random withdrawal key/nonce helpers are exported.

### Fixed
- Batch-create decoding rejects missing outcomes and inconsistent aggregate counts; unknown rejection enum values retain their numeric code.
- Realtime reconnects use capped exponential backoff with per-subscription jitter.
- Signing timestamps stay on wall clock during large concurrent bursts.
- Catalog hydration rejects conflicting symbol/asset identities atomically and accepts valid proto3 scale `0` values; REST and realtime public trades carry catalog quantity-scale metadata.

## 0.1.0a20

### Fixed
- ConnectRPC responses are capped at 4 MiB explicitly, including catalog hydration.
- The funded market roundtrip can use external order-book liquidity when dedicated maker credentials are unavailable and cancels only its own client order IDs.

### Testing
- Hardening coverage now injects corrupt and oversized protobuf catalog responses.
- Tests that use non-dry-run `CancelAll` require an explicit dedicated-account cleanup gate.

## 0.1.0a19

### Fixed
- Realtime HTTP 401/403 errors now expose `Code`, `Context`, and `Endpoint` in addition to the existing status/label/body fields.
- Realtime WebSocket messages and protobuf record/field lengths are capped at 8 MiB.
- Snapshot reconnect retries retain buffered publications across failed attempts; recovery-buffer overflow now fails closed instead of discarding the oldest publication.
- `SnapshotThenStream.Close` cancels an in-flight snapshot retry and is safe before `Start`.
- Every live-test and smoke path now uses `POLYESTER_TEST_TRADE_SYMBOL`; legacy smoke-symbol variables are no longer consulted.

### Testing
- Added oversized-message and combined reconnect/retry/cancellation fault-injection regressions.

## 0.1.0a18

### Breaking
- `WaitForCatalogs` returns an error when construction-time catalog hydration fails or catalogs are unusable (was Ok-after-fail). Use `CatalogsLastError()` to inspect. Order/trigger writes that auto-wait also surface that error.
- `codecs.FormatQtyScaled` / `codecs.FormatLedgerU128` now return `(string, error)` and reject scales above `MaxProtocolScale` (36) instead of allocating pathological pads.
- `catalogs.Manager.HydrateSpotConfig` / `HydrateZipperConfig` / `HydrateDepositWithdrawConfig` return `error` and reject oversized u64→u32 IDs/scales and scales > 36 (no silent truncation).
- Realtime HTTP 403 token responses map to `errors.AuthError` (with status, label, truncated body) instead of opaque `RealtimeError` strings matching `HTTP 403`.

### Fixed
- JSON-RPC client keeps `http.Client.Timeout` as the e2e deadline, caps response bodies at 1 MiB, and validates `jsonrpc=="2.0"`, matching `id`, and exactly one of `result`|`error`.
- `SnapshotThenStream` surfaces refresh errors via `Err()`, clears on success, and fail-closes after one bounded reconnect retry.
- Realtime `Subscription.Close` cancels promptly and waits for run-loop exit; `Client.Close` closes tracked subscriptions.

### Features
- `Orders.WaitForOrderTradesComplete` polls until sum of trade quantities equals order `cum_qty` or timeout.

### Testing
- Unit + httptest coverage for scale bounds, catalog reject, JSON-RPC envelope/size/timeout, realtime 403, WaitForCatalogs failure, and snapshot errors.
- Live harness unifies `POLYESTER_TEST_TRADE_SYMBOL`; market BUY→SELL roundtrip fixture carries filled qty into cleanup sell.

## 0.1.0a17

### Breaking
- `AssetBalance` drops `TradingUpdatedAtNs` / `FundingUpdatedAtNs` / `ReservedUpdatedAtNs`. Use `TradingRevision` (orders trading/reserved/available) and `FundingRevision` (orders funding independently) instead.
- `Manager.BaseQuantityScaleForSymbol` / `BaseQuantityScaleForSymbolID` return `(scale, ok)` and no longer invent scale `8` when unknown/unhydrated. Decode-only paths keep an explicit fallback of `8`.

### Fixed
- Order/trigger write paths wait for background catalog hydration before resolving pair quantity scale, preventing first-order false `INSUFFICIENT_FUNDS` when a pair (e.g. ETH-USDT scale 6) was encoded at invented scale 8.

## 0.1.0a16

### Fixed
- Realtime now negotiates the `centrifuge-protobuf` WebSocket subprotocol and uses binary, length-delimited Centrifugo commands, replies, pings, and publications. Previous releases selected `:proto` channels while speaking the JSON client protocol, so subscriptions could handshake but receive no binary publications.
- Concurrent authenticated calls now receive distinct monotonic signing timestamps, preventing identical same-millisecond requests from colliding with replay protection.
- BUY trailing-stop requests are rejected locally because the wire strategy is SELL-only; they are no longer silently encoded as SELL.
- Authentication failures without server detail now carry a non-empty fallback message.
- `Config` / `Credentials` `String`/`GoString` redact private key material.
- Realtime subscription-token HTTP exchange rejects response bodies larger than 64 KiB.
- Public ID parsing prefers canonical base58 when an all-digit string round-trips via `FormatID`.
- `QuantityScaleForSymbol` returns an error instead of silently defaulting to scale 8.
- Subscriptions expose `Resubscribes` / `TakeResubscribed` after reconnect gaps.
- `MarketTradesResult` exposes `NextPageToken` for public trades pagination.
- Candle subscriptions normalize aliases (`MIN_1` / `min1`) to the live channel label (`1m`).

### Changed
- Live integration soft-skips fail closed when `POLYESTER_TEST_STRICT_LIVE=1` (local release QA). Default CI remains unit-only (`go test ./...` without the `integration` build tag).

## 0.1.0a15

### Fixed
- `models.TriggersList` and `models.TriggerEventsList` now surface `NextPageToken` from list responses so trigger pagination can continue through the high-level wrappers

## 0.1.0a14

### Breaking
- Stable MFA auth error codes: `AUTH_API_KEY_MFA_REQUIRED` is removed; use `AUTH_MFA_NOT_ENROLLED`, `AUTH_STEP_UP_REQUIRED`, `AUTH_MFA_ELEVATION_REQUIRED`, and `AUTH_MFA_LAST_FACTOR_REQUIRED` from `AuthErrorDetail`
- Generated `*.pb.validate.go` outputs are no longer shipped with the SDK bundle
- Removed JWT/session-only auth admin/mutation wrappers that cannot work with API-key auth:
  - `Policies`: all unary List/Get/Create/Update/Delete/Set methods (subscribe-only remains)
  - `APIKeys.Create` / `Update` / `Delete` (List/Get/GenerateKeypair/Subscribe remain)
  - `SubAccounts` write/admin methods (Create/Update/Delete/SetMemberMFARequirement/RemoveMember/UpdateMemberRole/InviteMember/RespondInvite)
  - `AddressBook` entry/tag mutation methods (list/view/subscribe remain)
  - `Profile.Get` / `Update` / `GetUsernameHistory` (`SubscribeIdentity` remains)
  - `Resolve` / `Accounts` service removed entirely
- Dropped mutation request builders used only by the removed methods (`codecs` policy/API-key/subaccount/address-book write patches)

### Features
- `errors.IsMFAEnrollmentRequired` / `IsStepUpRequired` / `IsMFAElevationRequired` / `IsMFALastFactorRequired` classify MFA control flow from structured auth codes only (no message heuristics)
- Public method options expose `polyester.api.MFARequirement` documentation metadata
- `Policies.SubscribeAPIPolicies` typed subscribe for `private:auth:api-policies:{account}:proto` (parity with subaccount policies / other private auth streams)

### Testing
- Unit coverage for MFA auth-code mapping and predicates
- Unit coverage for API/subaccount policy realtime protobuf decode
- Integration: policy/API-key mutation realtime tests removed; private subscribe coverage remains

### Changed
- CI no longer auto-commits `sdk-capabilities.json` / README on pull requests. Capability refresh + optional bot commit runs only on merge to `main`.
- README capability matrix labels updated for API-key-safe auth surface (policies/profile subscribe; resolve unsupported)
- Realtime subscribe waits for the Centrifugo handshake (including private token fetch) before returning; initial auth failures are returned immediately

### Fixed
- API-key signatures for `WireFormat: "json"` now hash ProtoJSON body bytes (matching `connect.WithProtoJSON()`), not binary protobuf
- Conditional trigger children reject `post_only` for market / limit IOC / limit FOK (limit GTC only), matching Python/Rust and the CreateTrigger schema

## 0.1.0a13

### Breaking
- explicit execution variants: order/trigger create now encode onto typed execution oneofs. The flat public inputs (`order_type`/`tif`/`post_only`) are mapped to `OrderIntent` execution variants (`market`→`MarketIoc`, `limit`+`ioc`→`LimitIoc`, `limit`+`fok`→`LimitFok`, `limit`+`gtc`/default→`LimitGtc`); `post_only` is rejected for non-GTC executions
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
