# Changelog

## Unreleased

### Added
- `MarketData.GetSpotConfig` returns a typed `models.SpotConfig`: `Assets`
  (`models.SpotAsset`), `Pairs` (`models.SpotPair`: IDs, assets, scales, tick
  and step size, minimums, orderbook buckets, listing times, status labels
  such as `halted`, default slippage and reference drift), `TsSec` and the
  order limits. Each pair has `MaxLimitQtyBase`, `MaxMarketQtyBase` and
  `MaxOrderNotionalQuote` (decimal strings, empty when the pair has no
  maximum) and `MaxOpenOrdersPerPair`; the config has `MaxOpenOrdersTotal`.
  The open-order counts are the venue's defaults for a typical account, zero
  when not published; an account may have a lower limit of its own. `Raw`
  remains the proto-JSON payload `Catalogs.HydrateSpotConfig` accepts.
- `PairConstraints` exposes the same limits (`MaxLimitQtyBase` /
  `MaxLimitQtyScaled`, `MaxMarketQtyBase` / `MaxMarketQtyScaled`,
  `MaxOrderNotionalQuote`, `MaxOpenOrdersPerPair`) and `MaxQuoteSlippage` /
  `MaxQuoteSlippageTicks`, the largest absolute slippage cap an order
  accepts. `Catalogs.MaxOpenOrdersTotal()` returns the account-wide default.
  A malformed limit makes the constraints unavailable rather than guessed.
- `Order.InheritedCumQty`: the filled quantity a replacement order inherited
  from its predecessors; `CumQty` minus it is what that generation executed.
- `BatchReplaceAdmissionItem.ActionTaken` and
  `BatchReplaceStatusItem.ActionTaken`: `replaced` when a successor was
  admitted, `amended` for a cancel-only outcome with no successor.
- `APIError.ExistingOrderID` and `BatchCreateResultItem.ExistingOrderID` name
  the order already holding the client order ID on a
  `ERROR_CODE_CONFLICT_DUPLICATE_CLIENT_ORDER_ID` rejection, when known.
- `errors.TimestampSkewError` for the API-key `TIMESTAMP_SKEW` rejection,
  whether sent as a Connect error or as the gateway's JSON or problem JSON
  body (`{"code":"TIMESTAMP_SKEW"}`). It carries the HTTP status.
  `errors.IsRetryable` reports true: the request changed nothing and a retry
  signs it again.
- `errors.NotImplementedError` for Connect `unimplemented` and bare HTTP 501
  responses. A bare 404 is still `errors.RouteNotFoundError`.
- `LedgerTransfer.TransferID`: the unsigned 128-bit decimal transfer ID both
  legs of a transfer share.
- A trailing stop's `TriggerDetails.TriggerPrice` is its current threshold
  once armed.
- `realtime.SubscribeProtoBatch`, `services.SubscribePublicProtoBatch` and
  `services.SubscribeAccountProtoBatch` subscribe to channels whose
  publications batch events.

### Changed
- **Breaking:** `MarketData.SubscribeTrades`, `Trades.Subscribe`,
  `Transfers.Subscribe` and `Triggers.SubscribeEvents` decode the batched
  publications the realtime channels now send and emit each event in order.
  Previously a batch decoded as a single event and the rest were lost. The
  single-event decoders `decode.MarketTradeFromBytes`,
  `decode.UserTradeFromBytes`, `decode.LedgerTransferFromBytes` and
  `decode.TriggerEventFromBytes` are replaced by `MarketTradeBatchFromBytes`,
  `UserTradeBatchFromBytes`, `LedgerTransferBatchFromBytes` and
  `TriggerEventBatchFromBytes`.
- **Breaking:** `decode.SpotConfigFromProto` returns an error for a response
  it cannot decode instead of an empty config; `GetSpotConfig` returns it.
- **Breaking:** Connect `unimplemented` responses map to
  `errors.NotImplementedError` instead of `errors.APIError` with code
  `unimplemented`.
- `MarketOverview.CreateSubscription` with `Symbols` drops other markets from
  live and buffered publications too, not only from the snapshot.
- `Triggers.Modify` and `Triggers.Resume` no longer require a symbol. Pass nil
  for `symbol` and `symbolID` to target the trigger's own market; a symbol,
  when set, must be that market. An API key limited to specific markets must
  still pass it.
- A sell market order accepts `MaxSlippageBps` 1 to 9999; a buy still accepts
  10000. An attached trailing stop accepts 1 to 9999 on either side, and
  `Triggers.Modify` accepts 1 to 9999 (0 still clears). The backend rejects
  10000 on these with `MAX_SLIPPAGE_INVALID`.

### Internal
- `sdk-capabilities.json` no longer carries `publicDescriptorSha256` or `generatedAt`, so a proto publication that only changes `gen/` and `manifest.json` no longer makes it stale; `manifest.json` stays the single source of the descriptor hash.

## 0.2.0

Git tag: `v0.2.0`. Covers every change since `v0.1.0-alpha.56`.

### Versioning
- The Go, Python and Rust SDKs now share one version number and are released
  together, starting at `0.2.0`. Versions are plain SemVer: Go tags are
  `vMAJOR.MINOR.PATCH` (`v0.2.0`), with no `alpha`/`a` suffix. Until `1.0.0`
  a minor release may change the API; a patch release does not.
- Install with `go get github.com/Fabric-Labs/polyester-sdk-go@v0.2.0`. The
  repository is public, so `GOPRIVATE` / `GONOSUMDB` settings are no longer
  needed. The older `v0.1.0aNN` tags were never valid Go module versions;
  `v0.1.0-alpha.56` is the previous release.

### Added
- Attached take-profit, stop-loss and trailing-stop legs and standalone
  stop-loss, take-profit and trailing-stop triggers can watch the market's
  index price: set `RiskLegInput.TriggerPriceSource` or
  `TrailingStopInput.TriggerPriceSource`, or pass `triggerPriceSource` to
  `Triggers.Create`, as `"index"` (or `"last"`, the default when empty).
  Other values are refused with a validation error. Order reads now report
  each attached leg's `TriggerPriceSource` with the same labels, so a leg
  read back can be sent again unchanged.
- `CancelAllOrdersResult.ListingStatus`: `included`, or
  `not_retained_read_open_orders` on a retry with the same request ID within
  120 seconds, which returns the original counts with an empty
  `CancelledOrders`. An empty retried list does not mean nothing was
  canceled; read open orders for current state.
- `CancelAllAfterResult.EffectiveScope` (`models.DeadManSwitchScope`: account
  ID, symbol ID and side) describes the armed switch; nil when disabled.
- `GetOrderResult.ExecutionsComplete` is true when the order's trades across
  every page add up to its filled quantity, and false while a recent
  execution is still being recorded.
- `LifecycleFlowSummary.IsDelayed` is true when an open flow has made no
  progress for well beyond its expected step time.
- `realtime.OrderStale.OrderIDs` names the orders that are temporarily not
  current when `Reason` is `order_unavailable`; the rest of the view stays
  current and units keep arriving. A later `OrderStale` replaces the reason
  and IDs, and consumers that join a running subscription receive them.
- `errors.AuthCodeTemporarilyUnavailable` (`AUTH_TEMPORARILY_UNAVAILABLE`).

- Spot config pairs carry the venue's per-order limits in `SpotConfig.Raw`:
  `max_limit_qty_base`, `max_market_qty_base`, `max_order_notional_quote`
  (each absent when the pair has no maximum) and `max_open_orders_per_pair`.
  Orders past the open-order limit are rejected with
  `ERROR_CODE_MAX_OPEN_ORDERS_PER_PAIR`.

### Changed
- **Breaking:** `errors.IsRetryable` reports false for every order error code
  ending in `_OUTCOME_UNKNOWN` (`ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN`,
  `ERROR_CODE_ORDER_CANCEL_OUTCOME_UNKNOWN`,
  `ERROR_CODE_ORDER_MODIFY_OUTCOME_UNKNOWN`,
  `ERROR_CODE_TRIGGER_CREATE_OUTCOME_UNKNOWN`,
  `ERROR_CODE_TRIGGER_CONTROL_OUTCOME_UNKNOWN`). It reported true for the
  order cancel and modify codes in `v0.1.0-alpha.56`. The change may still
  apply, so a blind retry can place a second order: read the order or trigger
  back first. This matches the TypeScript SDK.
- `errors.MutationOutcomeUnknown` reports true for every order error code
  ending in `_OUTCOME_UNKNOWN`, read from the generated `orders.v1` codes, so
  a code added to the API is covered when the SDK is regenerated. New in this
  release: `ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN`,
  `ERROR_CODE_TRIGGER_CREATE_OUTCOME_UNKNOWN` (a create sent but not confirmed
  in time may have been accepted) and
  `ERROR_CODE_TRIGGER_CONTROL_OUTCOME_UNKNOWN` (a trigger cancel, pause,
  resume or modify).
- `errors.IsRetryable` reports true for `AUTH_TEMPORARILY_UNAVAILABLE`: the
  request changed nothing; retry the same request.

### Fixed
- The lifecycle reason `DEPOSIT_SOURCE_NOT_CONFIRMED` reads as
  `deposit_source_not_confirmed` instead of `unknown_reason_400`.
- `TestnetEnvironment` and `EnvironmentFromName("testnet")` now use
  `https://api.testnet.polyester.com` and `wss://api.testnet.polyester.com`.
  Earlier versions pointed at `api-testnet.polyester.com`, which does not
  exist, so every testnet call failed with a DNS error. The devnet preset and
  `transport.DefaultConfig` now use the documented `api.devnet.polyester.com`
  host instead of `api-devnet.polyester.ai`.
- The README no longer claims `VIP.GetVIPStatus` works with an API key: the
  API refuses API-key callers with `permission_denied`. VIP tiers, effective
  spot fees and trading rate limits do work with an API key.
- The `User-Agent` header reports this SDK's module version
  (`polyester-sdk-go/v0.2.0`). Earlier versions reported the version of the
  application that imported the SDK.
- README documentation links point at the public developer docs on
  `polyester.com`.

## 0.1.0-alpha.56

Git tag: `v0.1.0-alpha.56`.

### Changed
- Release tags now use Go module versions (`v0.1.0-alpha.56`), so
  `go get github.com/Fabric-Labs/polyester-sdk-go@v0.1.0-alpha.56` records
  that version. Earlier tags such as `v0.1.0a55` resolve to pseudo-versions.

### Fixed
- Trigger reads report a buy trailing stop's child as `OrderType` `limit`
  (time in force `ioc`) at its maximum price, as the server places it;
  v0.1.0a55 reported `market`.

## 0.1.0a55

Git tag: `v0.1.0a55`.

### Changed
- **Breaking:** `CreateOrderRequest.AttachedRisk` is `*models.AttachedRiskInput`
  instead of `map[string]any`; the map was always rejected.
- **Breaking:** `codecs.ModifyOrderToProto` takes a new `newAttachedRisk
  *models.AttachedRiskInput` parameter after `newQty`, and
  `codecs.BatchReplaceItemToProto` encodes `BatchReplaceItem.NewAttachedRisk`.
  Pass `nil` to keep an order's attached risk unchanged.
- **Breaking:** `Orders.Subscribe` uses the private WebSocket and returns
  `*realtime.Subscription[realtime.OrderStreamEvent]` instead of
  `*realtime.Subscription[models.Order]`. It returns at once and reports the
  handshake as `OrderStreamConnected`. The previous order realtime channels,
  including the batch-replace order channel, are retired; use
  `Orders.GetBatchReplaceStatus` and the order stream's revisions for
  batch-replace progress.
- **Breaking:** `decode.OrderFromBytes`, the decoder of the retired order
  realtime publications, is removed. Use `decode.OrderFromProto`.
- A sell `trailing_stop` create given a `limitPrice` is refused with a
  validation error instead of ignoring it: only a buy has a maximum price.
- Trigger create and modify refuse both `MaxSlippageTicks` and
  `MaxSlippageBps` together (previously one silently won), and a trailing
  stop's `MaxSlippageBps` is limited to 1 to 9999 on either side.
- **Breaking:** `codecs.ModifyTriggerOptions.LimitPrice` is removed: a
  trigger's child limit price is fixed at create, and the server refuses
  any change to it. Cancel the trigger and create a new one instead.
- **Breaking:** trading prices use 9 decimal places. `codecs.PriceTickScale`
  and the new `models.PriceTickScale` are 9: one price tick is 1e-9 quote
  units. This applies to order, trigger, trailing, activation, orderbook,
  trade, candle, market-overview and tick-size values. Raw tick values
  built for the previous 6-decimal scale are 1,000 times too small; scale
  them by 1,000 or pass decimal strings. Market-overview USD amounts stay
  at 1e6.
- **Breaking:** `MaxSlippageTicks` on market orders, triggers, trigger
  modifications and trailing-stop reads is `int64`, matching the 64-bit
  price-delta field in the API.
- Candle and market-overview base volume now use the base asset
  `market_data_volume_scale` from `GetSpotConfig`. Primary candle prices
  use the fixed 9-decimal price scale.
- Composite reference-candle OHLC uses the pair `reference_price_scale`.
  `GetCandlesWithReference` and `GetCandlesColumnsWithReference` request
  that series. `MarketOverviewEntry.Volume24HBase` is the decoded decimal
  when the catalog scale is known; `Volume24HBaseScaled` stays the integer.

### Added
- Orders accept attached take-profit, stop-loss, trailing-stop and OCO
  policies: `CreateOrderRequest.AttachedRisk`, the new `Orders.ModifyWith`
  (`ModifyParams.NewAttachedRisk`), and `BatchReplaceItem.NewAttachedRisk`
  take a typed `models.AttachedRiskInput`. Create and modify results expose
  the configured legs' trigger IDs.
- Trigger events label standalone trailing-stop activation as `activated`,
  and `TriggerEventTypeFromLabel` accepts `activated` as a filter.
- `realtime.Client.SubscribeOrders` streams an account's orders over the
  private WebSocket (`/v1/ws/private`, subprotocol `polyester.v1.proto`).
  It delivers typed `realtime.OrderStreamEvent` values: `OrderSnapshot`
  (the full live view, installed whole), `OrderReady` (the stream reflects
  every acknowledgement given before the subscribe; wait for it before
  trusting that a missing order does not exist; once per server
  subscription, so again after a resubscribe or reconnect), `OrderUnit`
  (changed orders, removed order IDs, and order events and command outcomes
  as `models.OrderUnitEvent`), `OrderStale`, `OrderStreamConnected`,
  `OrderStreamDisconnected` and `OrderStreamError`. Sequence gaps, expiring
  tokens, heartbeat loss and reconnects resume from the last applied
  position; subscriptions of the same account share one server
  subscription.
- `realtime.ErrClientClosed` is returned by `SubscribeOrders` (and
  `Orders.Subscribe`) after `Client.Close`. A `Close` that races a
  subscribe closes the new stream.
- `errors.RealtimeError` carries `Code` (the server's request error, for
  example `FORBIDDEN`) and `CloseCode` (for example 4403) for the private
  order stream.
- Standalone trailing stops can buy. `Triggers.Create` with
  `trigger_type` `trailing_stop` accepts `side` `buy`; its `limitPrice` is
  the buy's maximum price, the most the child pays per unit, encoded at 9
  decimal places. It is required for a buy, must be positive and at least
  the activation price. Trigger reads project a buy's maximum price as
  `Trigger.LimitPrice`; `Trigger.Side` and `Details.TroughPrice` show the
  side and the lowest price since arming.
- Stop-loss and take-profit market children carry a slippage cap:
  `CreateTriggerOptions.MaxSlippageTicks` or `MaxSlippageBps` is encoded on
  the market child (previously it was dropped and the pair default
  applied). At most one may be set; a buy accepts 1 to 10000 bps, a sell 1
  to 9999; a limit child refuses a cap. Get, list and the trigger stream
  expose it as `Trigger.MaxSlippage` / `Trigger.MaxSlippageBps`. A buy
  stop-loss or take-profit may use a market child.
- `Orders.CancelAll` returns `CancelledOrders` (order ID and client order
  ID, oldest first, at most 200), `TotalCancelled` and `Truncated`.
- `MarketData.GetTradesAfter` walks a symbol's trades gap-free by match ID
  and returns `CompleteThroughTradeID`, `NextAfterTradeID` and `EpochEnds`.
  Start at `"0"`; both watermarks are always decimal match IDs, `"0"`
  included, so an empty first page's cursor is valid for the retry.
- `VIPStatus.SettledVolumeQuality` and `AverageAopQuality` report how
  complete each rolling 30-day metric is.
- `errors.MutationOutcomeUnknown` reports true for
  `ERROR_CODE_ORDER_CANCEL_OUTCOME_UNKNOWN` and
  `ERROR_CODE_ORDER_MODIFY_OUTCOME_UNKNOWN`; `errors.IsRetryable` reports
  true for them and for `ERROR_CODE_ACCOUNT_REPAIRING`.

### Fixed
- Labels for enum values newer than this SDK version (for example a new
  trigger failure reason) read `unknown(<n>)` instead of panicking.
- Account-scoped subscriptions given no account ID now use the client's
  `DefaultAccountID` value instead of formatting its pointer.
- Order reads no longer report an attached-risk leg whose state is
  `not_configured`; the server sends that state for a leg the order
  does not have.

## 0.1.0a54

Git tag: `v0.1.0a54`.

### Added
- Market-order create/preview now expose `MaxSlippageBps` or
  `MaxSlippageTicks` and serialize the chosen override onto
  `OrderIntent.market_ioc`. Omit both to keep pair defaults. The fields
  are mutually exclusive, bounded (`bps` 1–10000, ticks positive), and
  rejected on limit orders.

## 0.1.0a53

Git tag: `v0.1.0a53`.

### Added
- Limit GTD orders: `tif="gtd"` plus required RFC3339 UTC `expires_at`
  encode `OrderIntent.limit_gtd` (price, optional `post_only`, 1s–30d
  expiry). Order reads expose `expire_at` for GTD and omit it otherwise.

### Changed
- Generated Connect/protobuf bindings now include
  `ClaimsService.GetDailyClaimStatus` and `ClaimDailyReward`. Those RPCs
  stay JWT/session-only and are not wrapped in this API-key SDK.

## 0.1.0a52

Git tag: `v0.1.0a52`.

### Breaking
- `chain.PolyesterTestnetEnvironment` is now public testnet
  (`api-testnet.polyester.com`, chain `888169`). The previous preset (devnet
  URLs, chain `888168`) is `chain.PolyesterDevnetEnvironment` and remains the
  client / chain-helper default.

### Added
- Paginated order lineage history: `Order.Lineage` / `UserTrade.Lineage`
  (`ID` + one-based `Generation`), `GetOrderResult.Transfers` /
  `NextPageToken`, and `UserTradesList.Transfers`. `Orders.Get` accepts
  optional `GetOrderOptions` (`IncludeExecutionHistory`, `Limit`,
  `PageToken`). `Trades.List` accepts optional `ListUserTradesOptions`
  (`OrderID` XOR `LineageID`, `ThroughGeneration`, `IncludeTransfers`).
  `WaitForOrderTradesComplete` pages execution history; an empty page is
  not a settlement watermark. `CumQty` / `AvgPx` are lineage-cumulative.
- First-class `Config.Environment` / `polyester.DevnetEnvironment` /
  `TestnetEnvironment`. `CreateEnvironment` and `Environment.WithURLs` support
  custom / VPC endpoints. `FromEnv` reads `POLYESTER_ENV=devnet|testnet` and
  optional `POLYESTER_API_URL` / `POLYESTER_WS_URL`.


## 0.1.0a51

Git tag: `v0.1.0a51`.

### Breaking
- `DepositWithdrawConfig.PolyesterChainID` is removed. The zipper config
  no longer publishes a dedicated Polyester chain id.

### Added
- `MarketOverview.GetCurrencyConversionConfig` and
  `GetCurrencyConversionRates` wrap the public currency-conversion RPCs.
  Fiat `UnitsPerUsdE8` is currency units per 1 USD at 1e8 scale (USD
  identity is 100_000_000). Stablecoin `UsdPerUnitE8` is observed USD
  per unit at the same scale. A missing fiat snapshot or omitted
  stablecoin is unobserved, not zero. Rates fail with unavailable
  (HTTP 503) before any observation exists. Fraction digits are
  presentation defaults, not rate precision.

### Changed
- CI compiles golangci-lint v2.6.2 with the module Go 1.26 toolchain. The
  prebuilt binary was built with Go 1.25 and cannot lint this module.
- Connect `PermissionDenied` / `Unauthenticated` now stamp `AuthError.Code`
  and `Status` (`permission_denied`/403, `unauthenticated`/401) so live
  helpers treat messages like `address book write denied` as a missing
  API-key scope instead of a hard failure.
- Document that a reused `ClientOrderID` on create is a conflict after the
  first admission (`CONFLICT_DUPLICATE_CLIENT_ORDER_ID`). Reconcile with
  `Get` / `ListOpen` before creating again.
- Bump `go-ethereum` to v1.17.5, `gnark-crypto` to v0.21.0, and
  `golang.org/x/sys` to v0.48.0 so the September 2026 dependency scan
  findings close. The module `go` line is now 1.26 because `x/sys`
  v0.48.0 requires it. `golang.org/x/crypto` dropped out of the module
  graph with the Geth upgrade (it was only pulled for unused SSH code).

## 0.1.0a50

Git tag: `v0.1.0a50`.

### Breaking
- `Orders.CancelAll` now sends repeated Connect `symbol_ids` instead of a
  single `symbol_id`. The existing `CancelAll` signature still accepts one
  optional display symbol. Use `CancelAllWith` for `symbols` or `symbol_ids`
  (mutually exclusive). An empty collection still matches all symbols; at most
  100 positive IDs are accepted and duplicates are ignored. Reuse `request_id`
  only when retrying identical cancellation criteria, and handle
  `ERROR_CODE_CANCEL_REQUEST_EXPIRED`.

### Added
- `MarketOverview.GetSpotVolumeHistory` wraps the public
  `GetSpotVolumeHistory` RPC (trailing 24-hour USD volume samples). Omit
  filters for every configured pair, or send up to 2,000 distinct positive
  pair IDs.
- Candle decode now preserves exact traded `quote_volume` decimal strings on
  `CandlePoint`, `Candle`, and columnar candle responses.
- Market overview rows expose optional `volume_24h_base_scaled`,
  `volume_24h_quote_scaled`, and canonical `volume_24h_usd_scaled` (1e6 USD
  scale). Absent values stay nil when the signed 64-bit amount or USD
  valuation is unavailable.
- Ladder trigger details decode `executed_levels` and `executed_qty`.
- TWAP market-IOC create can set per-slice `max_slippage_ticks` or
  `max_slippage_bps`.
- Connect withdraw and internal-transfer failures unpack the new
  `ErrorDetail` / `ErrorCode` types.

### Changed
- Generated Connect/protobuf bindings now include `AuthService.AcceptTerms`.
  That RPC stays JWT/session-only and is not wrapped in this API-key SDK.
  `AUTH_TERMS_NOT_ACCEPTED` is surfaced from Connect error details.

## 0.1.0a49

Git tag: `v0.1.0a49`.

### Changed
- Generated Connect/protobuf bindings now include
  `GetPortfolioEquityHistorySeries` and `GetPortfolioEquitySnapshot`. Those
  LedgerRead RPCs stay JWT/session-only (interactive root-account session)
  and are not wrapped in this API-key SDK.
- Equity history decode maps the additive `EquitySeries.portfolio_account`
  grouping (`account_id`, `remaining`) so the shared series type keeps
  portfolio identity instead of dropping it.
- Trigger modify proto validation is stricter: trigger price and trailing
  distance must be positive, and slippage / trailing BPS stay in 1–10000.

## 0.1.0a48

Git tag: `v0.1.0a48`.

### Fixed
- Qty/price strings with trailing zeros past scale are accepted when the
  value is exact after trim. `"1.500000000"` at scale 8 matches `"1.5"`;
  `"1.500000001"` still rejects.

## 0.1.0a47

Git tag: `v0.1.0a47`.

### Breaking
- `MeResult.APIKeyID` is the public `ak_` + 32-hex string from `MeResponse.api_key_id`,
  not a formatted numeric ID. It is empty when the call was not API-key authenticated.
- Subaccount `status` is decoded from `SubaccountStatus` (`active` / `disabled`;
  unspecified is empty). Invite list/get filters encode `incoming` / `outgoing`
  onto `SubaccountInviteDirection` and reject unknown labels.
- Cancel, cancel-all, cancel-all-after, and batch-cancel outcomes decode from
  closed proto enums onto the existing public labels (`accepted`, `submitted`,
  `dry_run`, `armed`, `disabled`, `rejected`). Unspecified or unknown values
  fail closed.

### Changed
- Generated Connect/protobuf bindings now include `GenerateUsernameOptions` and
  `ClaimGeneratedUsername`. Those Profile RPCs stay JWT/session-only and are
  not wrapped in this API-key SDK.

## 0.1.0a46

Git tag: `v0.1.0a46`.

### Added
- Attached-risk take-profit, stop-loss, and trailing-stop models expose typed
  per-leg runtime state: status, armed/terminal nanosecond timestamps, trigger
  id, and child order id.

### Fixed
- Batch create rejects duplicate non-empty per-item client order ids while
  continuing to allow omitted ids.
- Attached-risk decode retains stop loss when trailing stop is also present.
- Attached-risk decode constructs each typed leg from meaningful runtime state
  or a usable policy, including state-only responses; empty or unusable policy
  wrappers are omitted and absent policy fields remain empty.
- Stale/equal orderbook reset deltas no longer clear or rewind a newer snapshot.
- Trigger create/modify validates `trailing_distance_bps` in 1–10000, and
  ladder create requires resolved minimum ticks to be below maximum ticks.
- Client-side Connect signing failures remain `errors.AuthError` with their
  original typed cause instead of becoming a generic transport/API error.
- Client construction rejects non-absolute/non-HTTP API bases and API bases
  containing query/fragment delimiters, including bare `?` or `#`, while
  preserving previously supported HTTP(S) proxy URLs with userinfo. Formatted
  configuration omits URL userinfo and fails closed to a neutral placeholder
  for malformed URLs.
- Catalog hydration cycle-safely deep-clones caller-owned pointers, maps, and
  slices before publishing validated snapshots. Generic raw snapshots reject
  structs with unexported lock/no-copy state; `time.Time` remains explicitly
  supported.

## 0.1.0a45

Git tag: `v0.1.0a45`.

### Breaking

- `TradingRateLimitRule.Tier` is now `VIPTier` (`json:"vip_tier,omitempty"`).
- `TriggerEvent.Reason` is removed. Terminal reasons are typed
  `CancelReason` / `FailureReason` on both `Trigger` and `TriggerEvent`.
- `Trades.List` takes `afterMatchID *uint64`. When set, a resolved non-zero
  `symbol_id` is required.
- `AddressBook.GetView` takes `minimumViewRevision uint64` (0 omits the field).
  `AddressBookView` and `AddressBookViewInvalidation` expose `ViewRevision`.
- Connect is `symbol_id`-only except GetSpotConfig. Public methods still accept
  display `symbol` strings, but the SDK resolves them through the spot catalog
  and fails closed. Raw symbol filters are no longer forwarded on market
  overview, triggers list, cancel-all, or cancel-all-after.
- Order/trigger encode sets `SymbolId` and never `Symbol`. Create, preview,
  modify, and batch-create require a resolved non-zero `symbol_id`.
- `CancelAllOrdersToProto` / `CancelAllAfterToProto` take `symbolID *uint32`.
- Trigger `Modify` and `Resume` require `symbol` or `symbolID`.
- Policy models drop perp rules and removed fields (`perp_*`, daily limits,
  `global_notional_cap`, `max_open_positions`, `liquidation_only`,
  `daily_loss_limit`). `READ_TRANSFER_DESTINATIONS` is gone;
  `READ_ADDRESS_BOOK` exists. `SpotMarketRule` has `SymbolID` and optional
  display `Symbol`.
- Decode of market overview, fees, and triggers no longer reads proto
  `GetSymbol()`; display `Symbol` is empty unless filled from catalogs.

### Added

- `LedgerTransfer.Source` / `Destination` (`TransferSide`) including optional
  `ChainID` for external-address sides. This is the Zipper
  `ChainConfig.chain_id`, not an EIP-155 or Polyester chain id.
- `Trades.List` `afterMatchID` cursor (`after_match_id`). Requires a resolved
  non-zero `symbol_id`.
- Typed trigger `cancel_reason` / `failure_reason` labels on `Trigger` and
  `TriggerEvent` (proto oneof; unspecified/unset is empty).
- Address-book `view_revision` on `GetView` / invalidation, plus optional
  `minimumViewRevision` on `GetView`.

### Changed
- `Order.OrigQty` is documented as the current accepted total (updates after
  a successful modify). `BatchCreate` `request_id` is the batch idempotency
  key; item `client_order_id` remains optional.
- Trigger modify omits `activation_price` / `max_slippage_*` to preserve
  current values and sends explicit zero to clear them. Create/modify reject
  `max_slippage_bps` outside 1–10000; modify still accepts `0` to clear.

### Fixed
- Unary Connect `Content-Type` is `application/proto` rather than the
  streaming `application/connect+proto` value.

### Testing
- Compatibility coverage for unary content type, codec body bytes, and
  API-key signatures on binary and ProtoJSON wire formats.

### Docs
- README links the sibling
  [`polyester-examples-go`](https://github.com/Fabric-Labs/polyester-examples-go)
  cookbook near the top.
- README examples use placeholder `YOUR_ACCOUNT_ID` and `BTC-USDT`
  instead of a live Account ID and `BNB-USDT`.

## 0.1.0a44

Git tag: `v0.1.0a44`.

### Added
- Address-book write wrappers: `CreateEntry`, `UpdateEntry`, `DeleteEntry`,
  `CreateTag`, `UpdateTag`, and `DeleteTag`. Create and update accept
  `new_tags` for atomic tag create-and-attach; updates use a durable FieldMask
  (`label`, `note`, `tag_ids`, `new_tags`).
- `errors.AuthCodeInternalError` (`AUTH_INTERNAL_ERROR`).

### Changed
- Twitter social-verification handles are forwarded as-is, including an
  optional leading `@`.

## 0.1.0a43

Git tag: `v0.1.0a43`.

### Added
- VIP catalog and caller-root status: `client.VIP.ListVIPTiers` /
  `client.VIP.GetVIPStatus` (`vip.v1.VIPService`). Optional qualification
  metrics, timestamps, and next-tier thresholds stay omitted when unset.
- Effective spot fee rates: `client.Fees.GetSpotFeeRates`
  (`fees.v1.FeeService`), optionally filtered by subaccount and `symbol_id`.
- Public trading rate-limit catalog and authenticated account/API-key limits:
  `client.RateLimits.GetRateLimitConfig` /
  `client.RateLimits.GetTradingRateLimits` (`ratelimit.v1.RateLimitService`).
  `policy_class` uses full protobuf enum names.

### Changed
- SDK boundary cleanup: raw `symbol`/`symbols` filters are forwarded without
  catalog fail-closed checks (market overview, triggers list, cancel-all /
  cancel-all-after). `symbol_id` wire paths still resolve locally.
- Removed client-side pair-constraint preflight on order/trigger writes; the
  API remains authoritative for tick/step/minimum admission.
- Removed millisecond-shaped `*_ts_ns` response rejection; timestamps decode as
  returned by the server.

### Kept
- Pagination wire-safety (`PaginationLimit` / `ExplicitPaginationLimit`).
- `ResolveSymbolID` for `symbol_id` fields, Connect error mapping, secret
  redaction coverage, catalog zero-optional-minima hydration, realtime private
  auth tests.

## 0.1.0a42

Git tag: `v0.1.0a42`.

### Added
- Market overview `IndexPrice` from proto `index_price_ticks` (multi-venue index
  in quote units scaled by 1e6; zero when no fresh index is available).

## 0.1.0a41

Git tag: `v0.1.0a41`.

### Added
- Fail-closed catalog-backed symbol filters for market overview and raw symbol paths.
- Shared pagination validators before `int` → `uint32` conversions.
- Catalog pair-constraint accessors and deterministic order/trigger preflight.
- Shared `ts_ns` response-contract validation that rejects millisecond-shaped values.
- Focused regression tests for F001/F002/F004/F006/F007/F018–F020/F027/F008/F031.

### Fixed
- Unknown non-empty symbol filters no longer degrade into unfiltered requests.
- Connect validation/cancellation/deadline failures map into stable SDK error types.
- Zero-valued optional catalog minimums are treated as unset so live catalogs remain usable.

## 0.1.0a40

Git tag: `v0.1.0a40`.

### Added
- Public `RateLimitDetail` model for `polyester.ratelimit.v1` quota rejection
  payloads (`Reason`, presence-aware `Limit` / `Remaining` / `RetryAfterMs` /
  `PolicyVersion`, plus `OperationID`, `PolicyClass`, `Scope`, `RefillModel`).
- `errors.RateLimitError.Detail` carries that structured payload when Connect
  attaches top-level `RateLimitDetail` or nested `orders.v1.ErrorDetail.rate_limit`.
  `RetryAfter` prefers `detail.retry_after_ms`, then `Retry-After` /
  `Retry-After-Ms` / `Grpc-Retry-Pushback-Ms` headers.
- Preview / batch create / batch replace / batch cancel rejections surface
  `RateLimit` on `OrderErrorDetail` and batch result items when present.

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
- README `Supported surface` table is generated from `sdk-capabilities.json` (`--write-capabilities`); links to the public [SDK capability matrix](https://polyester.com/docs/developer-docs/getting-started/sdk-capability-matrix)
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
