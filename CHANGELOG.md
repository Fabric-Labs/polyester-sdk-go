# Changelog

## 0.13.0

Git tag: `v0.13.0`. Released 2026-11-08. Built on public contract `c8d9827`.

The `c8d9827` contract changes (received transfers in flow lists, receive windows, and the replay-capacity, receive-window and request-deadline refusals) are available on devnet with its next deployment, and on testnet with its next release.
- **Breaking:** `TriggersService.Create` refuses a TWAP with a market child whose `CreateTriggerOptions.MaxSlippageTicks` is zero or negative with `errors.ValidationError`, before anything is sent, as every other create-time tick cap already did. Setting both `MaxSlippageTicks` and `MaxSlippageBps` on that child is refused with the shared message `set at most one of max_slippage_ticks or max_slippage_bps`. Migration: send a positive tick cap, or leave `MaxSlippageTicks` nil for the pair default.
- **Breaking:** A REST HTTP 403 problem with a contract code that refuses the operation, such as `SUBACCOUNT_READ_FORBIDDEN` or `QUICKSWAP_NOT_PERMITTED`, keeps that code in `errors.AuthError.Code`, the problem's detail as its message and the problem in `Problem`, as a 401 or a credential code already did. The error was an `*errors.AuthError` with `Code` `permission_denied` and the message `403 Forbidden`. This applies to every service method. Migration: compare `AuthError.Code` with the contract code instead of `permission_denied`.
- **Breaking:** `InternalTransferResult`, returned by `InternalTransfersService.Create`, gains `Status`: `accepted`, `rejected` or `failed`, as the service answered the request; empty when the response names none, and `UNKNOWN(n)` for a status this version does not know. Receipts that differed only in their status read the same before. `accepted` means the transfer has a durable transfer ID, not that it has settled. Migration: a positional (unkeyed) `models.InternalTransferResult` struct literal no longer compiles; use keyed fields, or supply `Status` as the sixth value.
- **Breaking:** `Heatmap.SubscribeLive` now subscribes to the channel the server publishes live buckets on, `public:spot:market:heatmap:<1s|1m|5m|1h>:<symbol_id>:proto`. It used the interval's enum name (`...:heatmap:INTERVAL_1S:...`), which the server acknowledged but never published on, so every interval received nothing. An interval other than `1s`, `1m`, `5m`, `1h` or an enum name such as `INTERVAL_1S` is now refused with `*errors.ValidationError` before connecting, instead of subscribing to a channel that never delivers. `Heatmap.Get` now refuses `INTERVAL_UNSPECIFIED` the same way instead of sending it. Added `codecs.HeatmapIntervalChannelLabel` and `codecs.ResolveHeatmapInterval`. Migration: pass one of those intervals; code that subscribed to the lowercase channel directly as a workaround can use `SubscribeLive` again.
- **Breaking:** `Heatmap.Get` requests the depth you pass when it is a supported depth (1, 5, 10, 20, 50, 100, 200, 500 or 1000), and any other depth up to 1000 as the smallest supported depth covering it, the rule `Orderbook.Get` follows. It requested 5 for 0 to 5, so 1 returned 5 levels, and 200 for anything above 100, so 500 and 1000 returned 200 levels. 0 now requests 50 instead of 5. A depth below 0 or above 1000 is now refused with `*errors.ValidationError` before any request, instead of requesting 5 or 200. The response's `depth` names the depth requested. `codecs.DepthToProtoName` is replaced by `codecs.HeatmapDepth`, which returns that depth or the refusal. Migration: pass a depth from 0 to 1000, read `depth` from the response for the depth you got, and call `codecs.HeatmapDepth` instead of `codecs.DepthToProtoName`.
- **Breaking:** `models.MarketOverviewEntry` gains `Change24hBps`, the 24-hour change of the last price in basis points, and `Sparklines`, a list of `models.MarketOverviewSparkline` (`Interval`, such as `"SPARKLINE_24H"`, and `ClosePrices`, newest first as the server sends them), from `MarketOverview.List`, `MarketOverview.Subscribe` and `MarketOverview.CreateSubscription` (and `decode.MarketOverviewEntryFromProto`, `decode.MarketOverviewListFromProto` and `decode.MarketOverviewBatchDecoder`). The server sent both, and sent sparklines whenever `includeSparklines` was set, but they were dropped. A managed subscription keeps the snapshot's sparklines when a live update, which carries none, replaces a row. Rows encode `change_24h_bps`, and `sparklines` when present, as JSON. Added `decode.MarketOverviewSparklineFromProto`. An unkeyed `models.MarketOverviewEntry{...}` literal no longer compiles. Migration: name the fields in any `MarketOverviewEntry` literal you build yourself.
- **Breaking:** `Heatmap.Get` with `fromTsSec` and an explicit `end` now ends the first page at `end`. It ended it at the current time and sent `end` only on the continuation, so the server refused the continuation with "page_token does not match request" and the first page ran past the end you asked for. Migration: none, unless you relied on `fromTsSec` ignoring `end`; pass a nil `end` to read up to now.
- **Breaking:** `models.ExportGuardSignerWalletResult.EncryptedPrivateKey` and `models.Ed25519Keypair.SecretKeyHex` and `SecretKey` are now `models.Secret` instead of `string` and `[]byte`, so no printing, logging or encoding path reveals the private key they hold. Formatting the result or the key pair with `fmt`, as a value, a pointer or inside another struct (including an unexported field), with any verb, printed the export result's key, and the key pair's key through `%d`, `%t` and unexported fields; `encoding/json`, `encoding/xml` and `log/slog` wrote both keys, and gob encoded them. They now print and encode `[REDACTED]`, and gob refuses them. `Ed25519Keypair` JSON keeps `public_key_hex` and writes `"secret_key_hex":"[REDACTED]"`; it carried the key on purpose before. The type `auth.Secret` moved to `models.Secret` so models can hold it; `auth.Secret` is the same type and `auth.NewSecret` still builds it, and the gob refusal now reads "Secret cannot be gob-encoded". Affected: `GuardSigner.ExportWallet` and `decode.ExportWalletFromProto`; `APIKeys.GenerateKeypair` and `auth.GenerateEd25519Keypair`. Migration: read the keys explicitly with `result.EncryptedPrivateKey.Reveal()`, `kp.SecretKeyHex.Reveal()` and `[]byte(kp.SecretKey.Reveal())`, pass `kp.SecretKeyHex` straight to `Config.APIPrivateKey`, and write `models.NewSecret(value)` in struct literals; store a key you export on purpose from `Reveal`, not by marshalling the whole value.
- **Breaking:** Realtime errors no longer carry the user name or password of a URL. When the API refused a connection or subscription token with HTTP 401 or 403, `errors.AuthError.Endpoint` and the error text held the token URL with the API URL's userinfo, password included. They now hold that URL without userinfo, and keep the status, code and the rest of the endpoint. A token request that failed in transport returned the standard client's error, which masks the password but keeps the user name (`fake-user:***@`); it now names the URL without userinfo and keeps the cause, so `errors.Is` and `errors.As` still see it. A URL that does not parse no longer appears with its userinfo in the error either: the standard parser quotes the whole URL, so such an API or WebSocket URL now shows as `[REDACTED INVALID URL]`. REST errors already left the userinfo out. Affected: every private `Subscribe` (`APIKeys.Subscribe`, `AddressBook.Subscribe`, `AddressBook.SubscribeViewInvalidations`, `Balances.Subscribe`, `Lifecycle.SubscribeOpenFlows`, `Orders.Subscribe`, `Policies.Subscribe`, `Policies.SubscribeSubaccountPolicies`, `Policies.SubscribeAPIPolicies`, `SubAccounts.Subscribe`, `SubAccounts.SubscribeAPIKeys`, `Trades.Subscribe`, `Transfers.Subscribe`, `Triggers.Subscribe` and `Triggers.SubscribeEvents`), `realtime.Client.SubscribeOrders`, and `realtime.SubscribeProto`, `realtime.SubscribeProtoBatch` and `realtime.SubscribeProtoWithOptions` for any channel. Migration: none, unless you matched `AuthError.Endpoint` against a URL that included userinfo; compare it with the URL without userinfo.
- **Breaking:** `Lifecycle.ListFlows` with an owner account or a smart account address now also returns the transfers that account received, such as a transfer from another user or from another sub-account of the same main account, or a reward payout. It used to return only the flows the account owns. A page token is now bound to the request it was returned for: sent with a different account, participation, scope or order it is refused with `errors.ValidationError`, and tokens issued before this change are refused. `models.LifecycleFlowSummary` gains `Participation` (`"owner"` or `"recipient"` on a list that selects an account, `"unspecified"` otherwise), `SourceAddress` and `DestinationAddress`, from `Lifecycle.ListFlows`, `Lifecycle.ListFlowsWith`, `Lifecycle.GetFlow`, `Lifecycle.SubscribeOpenFlows`, `Lifecycle.SubscribeFlowDetail`, `decode.FlowsListFromProto` and `decode.FlowSummaryMessageFromProto`; `Lifecycle.GetFlowByTx`, `Lifecycle.ListFlowsByTx`, `decode.FlowFromGetByTxResponse` and `decode.FlowsByTxListFromProto` fill the two addresses too, and leave `Participation` empty. Added `Lifecycle.ListFlowsWith` and `services.ListFlowsParams`, whose `Participation` (`"owner"` or `"recipient"`) narrows a list that selects an account; an unknown value returns `errors.ValidationError` before anything is sent. Migration: for the previous owner-only list, call `Lifecycle.ListFlowsWith` with `Participation` `"owner"`; for a transfer, read the counterparty from `DestinationAddress` when `Participation` is `"owner"` and from `SourceAddress` when it is `"recipient"`; use keyed `models.LifecycleFlowSummary` literals, since the struct gains `Participation`, `SourceAddress` and `DestinationAddress`; and restart pagination from the first page instead of reusing a stored page token.
- **Breaking:** An order or trigger write whose receive window (10 seconds without `Config.RecvWindow`) ends before it is sent on for admission is now refused with `ERROR_CODE_REQUEST_DEADLINE_EXCEEDED` (308; REST 400 `REQUEST_DEADLINE_EXCEEDED`, Connect `failed_precondition`) and changes nothing. Such a request used to be processed late. It returns an `errors.APIError` with `Code` `errors.CodeRequestDeadlineExceeded`, which `errors.IsRequestDeadlineExceeded` reports. It is never an unknown outcome, and `errors.IsRetryable` now reports true for it. Affected: `Orders.Create`, `Cancel`, `Modify`, `ModifyWith`, `CancelAll`, `CancelAllWith`, `CancelAllAfter`, `BatchCreate`, `BatchCancel` and `BatchReplace`; `Triggers.Create`, `Cancel`, `Pause`, `Resume` and `Modify`; and `StopAllTrading`. Migration: handle it like `errors.TimestampSkewError`: send the request again, which signs it with a fresh timestamp.
- **Breaking:** `Config.RecvWindow` sets the receive window of every signed API request, sent as the signed `X-API-RECV-WINDOW` header: the server processes a request only until its signing timestamp plus the window. It must be whole milliseconds from 1ms to 60s, and `polyester.New` refuses any other value with `errors.ValidationError`. Zero, the default, sends no header, and the server applies 10 seconds as before. Realtime token requests always use the default. Added `auth.SignRequestWithRecvWindow`, `auth.CanonicalSigningStringWithRecvWindow`, `auth.ValidateRecvWindowMS`, `auth.MinRecvWindowMS` and `auth.MaxRecvWindowMS` for requests you sign yourself; the window's decimal string is signed on its own line right after the timestamp. `transport.Config.RecvWindowMS` and `transport.APIKeyInterceptor.RecvWindowMS` carry it. Migration: use keyed `Config`, `transport.Config` and `transport.APIKeyInterceptor` literals, since each gains a receive-window field.
- `errors.AuthCodeRecvWindowInvalid` and `errors.IsRecvWindowInvalid` name `AUTH_RECV_WINDOW_INVALID` (REST 401 `RECV_WINDOW_INVALID`, Connect `unauthenticated` with `auth.v1.AuthErrorDetail`): an `X-API-RECV-WINDOW` header that is not one whole number from 1 to 60000, empty, or repeated. It is an `errors.AuthError`, like the other API-key refusals, and not retryable without fixing the window.
- **Breaking:** `errors.AuthCodeReplayCapacityExhausted` and `errors.IsReplayCapacityExhausted` name `AUTH_REPLAY_CAPACITY_EXHAUSTED` (REST 429 `REPLAY_CAPACITY_EXHAUSTED`, Connect `resource_exhausted` with `auth.v1.AuthErrorDetail`), which every service that checks signed requests can answer: the API key, or its account, holds its full share of the record that keeps signed requests from being replayed until their receive windows end. It returns `errors.RateLimitError` with that `Code`, which `errors.AuthErrorCode` reads, and `errors.IsRetryable` reports true: the credentials are valid, the request was not processed, and the share frees as earlier windows end. It carries no `Retry-After`, so `RetryAfter` is nil unless the response named one: back off briefly and send again, which signs again; a shorter `Config.RecvWindow` makes room sooner. It used to be an `errors.APIError` that was not retryable. Migration: handle `AUTH_REPLAY_CAPACITY_EXHAUSTED` as a retryable `*errors.RateLimitError` (back off and send again, which signs with a fresh timestamp) instead of a terminal `*errors.APIError`.
- **Breaking:** A consumer that subscribes to an account's order stream while that stream is reconnecting now gets `realtime.OrderStreamConnected` first, then the cached `OrderSnapshot`, `OrderReady` and `OrderStale`, as a consumer joining a connected stream does. It got the cached snapshot (and ready and stale) at once, and `OrderStreamConnected` only after the reconnect, so a consumer that set up its state on `Connected` received orders before it. Affected: `Orders.Subscribe` and `realtime.Client.SubscribeOrders`. Migration: none, unless you read the snapshot before `OrderStreamConnected`; set up state on `OrderStreamConnected` and apply the snapshot that follows it.
- **Breaking:** A REST refusal whose code belongs to the transfer, withdraw or ledger service now reads with the `ERROR_CODE_` prefix in `*errors.APIError`'s `Code`, as its typed ConnectRPC detail already did and as order codes already did over REST. `InternalTransfers.Create` refused with `SAME_SOURCE_DESTINATION` over REST and `ERROR_CODE_SAME_SOURCE_DESTINATION` over ConnectRPC; both now read `ERROR_CODE_SAME_SOURCE_DESTINATION`. Likewise `ERROR_CODE_AMOUNT_BELOW_MINIMUM` from the `Withdraw` writes (`CreateToFunding`, `CreateToExternalChain`, `CreateAPIKeyToFunding`, `CreateAPIKeyToExternalChain`, `CreateWalletTradingWithdraw` and `SubmitPrepared`) and `ERROR_CODE_INVALID_SUBACCOUNT_ID` from `Balances.List`, `GetBalanceHistory`, `GetEquityHistory` and `Transfers.List`. A code no service's `ErrorCode` names stays as sent, and so does a transfer or withdraw code the auth service also names, `INVALID_REQUEST`, which REST can't attribute, so `errors.AuthErrorCode` still reads `AUTH_INVALID_REQUEST`; `*errors.ServerError`, `*errors.ValidationError` and `*errors.AuthError` keep the REST code as sent, as before. Added `wire.ErrorCodeFromREST`. Migration: compare against the `ERROR_CODE_` form, or strip the prefix before comparing.
- **Breaking:** A market name and a numeric market ID given together must name the same market, or the call returns `*errors.ValidationError` (`… symbol mismatch: "LTC-USDT" does not match symbol_id 1 (BTC-USDT)`) before anything is sent. The ID was sent and the name ignored, so `Triggers.Modify` with `LTC-USDT` and the ID of BTC-USDT changed a BTC-USDT trigger. A name the catalog maps to another ID, or that is not the catalog's name for the ID, is refused; without catalogs, or for an ID and a name the catalog doesn't know, the ID is sent as before. A name alone, an ID alone, or both naming one market are unchanged. Affected: `Triggers.Modify`, `Triggers.Resume`, `Trades.List`, `MarketData.GetTrades`, `GetTradesAfter`, `GetCandles`, `GetCandlesWithReference`, `GetCurrentCandle`, `GetCandlesColumns`, `GetCandlesColumnsWithReference`, `SubscribeTrades` and `SubscribeCandles`, `Heatmap.Get` and `Heatmap.SubscribeLive`, and `services.ResolveSymbolID` and `services.ResolveOptionalSymbolID`. The order writes already refused it; their message now names the symbol and ID. Migration: pass the name or the ID alone, or both for the same market.

## 0.12.0

Git tag: `v0.12.0`. Released 2026-11-07. Built on public contract `1bf038e`.

- `models.ChangelogPostSummary` (and so `models.ChangelogPost`) gains `Revision` and `UpdatedAt`, from `Changelog.ListPosts`, `Changelog.Posts` and `Changelog.GetPost` (and `decode.ChangelogPostSummaryFromProto`, `decode.ChangelogPostsPageFromProto` and `decode.ChangelogPostFromProto`). A published post can now be republished, for example to correct its copy or withdraw a change: `Revision` starts at 1 and increases with each republication, and `UpdatedAt` is when the current revision was published. Its `Slug`, `PublishedAt` and change IDs stay the same. A server that predates revisions sends neither: `Revision` reads 0 and `UpdatedAt` nil. To follow edits, read a post again when its `Revision` differs from the one you hold.
- **Breaking:** `Changelog.Changes` yields each change ID once per walk, and `Changelog.BreakingChangesSince` and `Changelog.ActionRequiredSince` return each change once. When a post is republished while a walk pages through it, the next page restarts at that post's first change, so changes already received came again and were returned twice. `Changes` skips the repeat; the two helpers keep the change at its first position with the wording last received. The helpers still filter on first publication, which a republication does not move, so they don't report an edit to an older post. Migration: none, unless you counted repeats; compare `Revision` from `Changelog.Posts` to find edited posts.
- **Breaking:** A REST `RATE_LIMIT_EXCEEDED` problem now fills `errors.RateLimitError.Detail` from its `rate_limit` object, for every call, whichever side refused. Every field is optional: the gateway's own refusal sends only `Reason` and `Scope` (`CLIENT_IP`, with no `OperationID`), and a service's also names the operation, the policy class and the remaining quota. An integer field that is not ASCII digits within the unsigned 64-bit range, such as `"²"` or a 5000-digit string, reads as absent. `RetryAfter` now prefers the problem's `retry_after_ms` over the `Retry-After` header, as for a ConnectRPC `RateLimitDetail`, and stays nil when the server gave no hint, as for a refusal of too many concurrent requests. It used to drop the object and read only `Retry-After`. The gateway's ConnectRPC refusal (`resource_exhausted`, "rate limit exceeded, retry later") still has no detail and falls back to `Retry-After` when present. A 429 problem with another code, such as `AUTH_RESOURCE_EXHAUSTED`, stays an `errors.APIError` that waiting does not resolve. Migration: wait for `RetryAfter` when it is set, which can now be shorter than `Retry-After`, and back off on your own schedule when it is nil.
- `errors.CodeRequestTooLarge` and `errors.IsRequestTooLarge` name the uniform HTTP 413 refusal (problem code `REQUEST_TOO_LARGE`) that every service now answers for a request body over its size limit, before authenticating it, on known-length and chunked bodies alike. Ledger reads (balances, balance and equity history, transfers) answered `PAYLOAD_TOO_LARGE` instead until server release 9.2.6, so match `REQUEST_TOO_LARGE` from then on. It was already an `errors.APIError` with that code and its `Problem`, which is not retryable and leaves no unknown outcome: reduce the body to fit the limit. A ConnectRPC message over the limit once decoded keeps its Connect `resource_exhausted` answer.
- `errors.AuthCodeWalletAlreadyLinked` and `errors.IsWalletAlreadyLinked` name `AUTH_WALLET_ALREADY_LINKED`, which `AuthErrorCode` reads from the ConnectRPC `auth.v1.AuthErrorDetail` (`already_exists`) and the REST 409 problem alike: a valid wallet login whose wallet is already linked to another account. It is a terminal `errors.APIError`, never retryable; log in with the account address the wallet already uses.
- `Orders.GetBatchReplaceStatus`, `Orders.WaitBatchReplace` and `Orders.BatchReplace` document the 60-second status window: the status is readable as soon as `BatchReplace` returns, `UNAVAILABLE` (`errors.ServerError`, retryable) means retry the same read within the window, and not found never means a rejected replacement. After the window, reconcile the individual orders and never send an accepted batch again. An item's `UpdatedTsNs` is an observation time that can be a later recovery time. `WaitBatchReplace` still polls through not found within the window, for APIs that answered it shortly after admission.
- **Breaking:** An account or sub-account ID that decodes to 0 now returns `*errors.ValidationError` before anything is sent, whatever form it arrives in. `"1"` is the public ID of 0, and `"0"`, `"00"` and the same with spaces read as 0. Such a sub-account ID was sent as 0, which the server reads as unset, so the call addressed the main account instead of the sub-account named; such an account ID subscribed to the channel of account 0. This covers the `subAccountID` argument, an `account` string or `{"subaccountId": ...}` / `{"sub_account_id": ...}` dict, a request's `SubAccountID`, the client's `DefaultSubAccountID` and `DefaultAccountID`, a Subscribe `accountID`, an internal transfer's destination account or sub-account, and a flow list's owner account. A Subscribe `accountID` that isn't a valid ID at all, such as `"acct_test"`, is now refused the same way instead of naming a channel no account has. Omitting the sub-account, or passing `""`, still addresses the main account. Affected: `APIKeys.List`; `AddressBook.ListEntries`, `ListTransferCounterparties`, `ListTransferDestinations`, `ListInternalTransferWhitelistEntries`, `GetWithdrawWhitelistView`, `GetView`, `CreateEntry` and `CreateTag`; `Balances.List`, `GetBalanceHistory` and `GetEquityHistory`; `Deposit.ListAddresses` and `CreateAddress`; `Fees.GetSpotFeeRates`; `GuardSigner.CreateWallet`, `GetStatus`, `SignProtectedAction`, `BatchSignProtectedActions`, `RotateWallet` and `ExportWallet`; `InternalTransfers.Create`; `Lifecycle.ListFlows`; `Orders.Create`, `Preview`, `ListOpen`, `ListHistory`, `Get`, `Cancel`, `Modify`, `ModifyWith`, `CancelAll`, `CancelAllWith`, `CancelAllAfter`, `BatchCreate`, `BatchCancel`, `BatchReplace`, `GetBatchReplaceStatus`, `WaitBatchReplace` and `WaitForOrderTradesComplete`; `RateLimits.GetTradingRateLimits`; `StopAllTrading`; `SubAccounts.Get`, `ListMembers` and `ListActivity`; `Trades.List`; `Transfers.List`; `Triggers.List`, `Get`, `Create`, `Cancel`, `Pause`, `Resume`, `Modify` and `ListEvents`; `Withdraw.CreateWalletTradingWithdraw` and `TradingWithdraws.CreateWalletTradingWithdraw`; every private Subscribe (`APIKeys.Subscribe`, `AddressBook.Subscribe`, `AddressBook.SubscribeViewInvalidations`, `Balances.Subscribe`, `Lifecycle.SubscribeOpenFlows`, `Orders.Subscribe`, `Policies.Subscribe`, `Policies.SubscribeSubaccountPolicies`, `Policies.SubscribeAPIPolicies`, `SubAccounts.Subscribe`, `SubAccounts.SubscribeAPIKeys`, `Trades.Subscribe`, `Transfers.Subscribe`, `Triggers.Subscribe` and `Triggers.SubscribeEvents`); and the helpers and codecs they use: `services.ResolveAccountID`, `services.SubscribeAccountProto`, `services.SubscribeAccountProtoBatch`, `services.ScopedSubAccount.ApplyOptionalSubaccountID`, `services.ScopedSubAccount.ApplyOptionalSubaccountIDPtr`, `codecs.ParseOptionalSubaccountID`, `codecs.CreateOrderToProto`, `codecs.PreviewOrderToProto`, `codecs.ModifyOrderToProto`, `codecs.CancelAllOrdersToProto`, `codecs.CancelAllAfterToProto`, `codecs.BatchCreateOrdersToProto`, `codecs.BatchCancelOrdersToProto`, `codecs.BatchReplaceOrdersToProto`, `codecs.CreateTriggerToProto` and `codecs.ModifyTriggerToProto`. Added `codecs.ParseAccountID`, which decodes an account or sub-account ID and refuses 0. Migration: to address the main account, omit the sub-account ID (nil or `""`), or pass `account` `"main"`, instead of passing `"1"` or `"0"`, and pass the public ID of a real account wherever an account ID is required.

## 0.11.1

Git tag: `v0.11.1`. Released 2026-11-07. Built on public contract `748066a`.

- A retried read whose context ends just as the server answers `unavailable` now returns the context's error, for which `errors.Is(err, context.Canceled)` or `errors.Is(err, context.DeadlineExceeded)` is true. It returned the server's retryable answer, for example `*errors.ServerError` "catching up", although that answer would have been retried and only the caller ended the read. Affected: `Triggers.Get`, `Triggers.List` and `Triggers.ListEvents`. A definite answer, such as not found, is still returned as the answer.

## 0.11.0

Git tag: `v0.11.0`. Released 2026-11-07. Built on public contract `748066a`.

- **Breaking:** An integer account ID passed to a private `Subscribe` now addresses the account it names. The `accountID any` argument formatted an integer as decimal text, which reads as a public ID, so 42 became `"42"`, the public ID of account 175, and the subscription named account 175's channel. An integer is now encoded as its public ID, as `codecs.FormatID` does (42 is `"j"`), and 0, a negative value, a `bool`, a `*big.Int` outside 1 to 18446744073709551615 and any other non-string, non-integer value, such as a float, return `*errors.ValidationError` before connecting. Public ID strings are used as before, and nil or `""` still selects the client's default account. Affected: `ApiKeys.Subscribe`, `AddressBook.Subscribe`, `AddressBook.SubscribeViewInvalidations`, `Balances.Subscribe`, `Lifecycle.SubscribeOpenFlows`, `Orders.Subscribe`, `Policies.Subscribe`, `Policies.SubscribeSubaccountPolicies`, `Policies.SubscribeAPIPolicies`, `SubAccounts.Subscribe`, `SubAccounts.SubscribeAPIKeys`, `Trades.Subscribe`, `Transfers.Subscribe`, `Triggers.Subscribe` and `Triggers.SubscribeEvents`, and the helpers they use: `services.ResolveAccountID`, `services.SubscribeAccountProto` and `services.SubscribeAccountProtoBatch`. The exported `services.ScopedSubAccount.ResolveSubAccountID` encodes an integer sub-account ID the same way when you call it directly; the SDK's own methods pass it a `*string`. Migration: an integer now subscribes to a different account, the one it names, so check any code that passed an integer; to keep addressing the account you reached before, pass its public ID string (`"42"` stays account 175).
- **Breaking:** `Orders.WaitForOrderTradesComplete` now reports completion only once the order has ended (`filled`, `canceled` or `rejected`) and its trades add up to `CumQty`, the rule the Python and Rust SDKs follow. It returned as soon as the trades recorded so far added up to `CumQty`, so a partially filled order that was still working, or a working order with no fills yet, could be reported complete while more fills were still to come. Such an order now keeps the wait polling until it ends or the timeout passes. Migration: call the wait once the order is expected to end (an IOC or FOK order, or after a cancel), and to check the trades recorded so far on an order that is still working, read it with `Orders.Get` and compare its trades to `CumQty`.
- **Breaking:** A decimal order quantity that doesn't fit the signed 64-bit wire field is now refused with `*errors.ValidationError` ("qty exceeds int64 range") before anything is sent. On a market with base quantity scale 18, a quantity of 10 scales to 10000000000000000000, which wrapped and was sent as -8446744073709551616 on both the binary and JSON wires. An oversized `max_quote_debit_scaled` (and `codecs.ResolveQuoteQtyScaled`) was already refused, but with a misleading "must be positive" message. Affected: `Orders.Create`, `Orders.Preview`, `Orders.BatchCreate`, `Orders.Modify`, `Orders.ModifyWith`, `Orders.BatchReplace` and `Triggers.Create`, for `qty`, `new_qty` and `max_quote_debit_scaled`, and the codecs they use: `codecs.ResolveQtyScaled`, `codecs.ResolveOptionalQtyScaled`, `codecs.ResolveQuoteQtyScaled`, `codecs.OrderIntentToProto`, `codecs.CreateOrderToProto`, `codecs.PreviewOrderToProto`, `codecs.BatchCreateOrdersToProto`, `codecs.ModifyOrderToProto`, `codecs.BatchReplaceItemToProto`, `codecs.BatchReplaceOrdersToProto` and `codecs.CreateTriggerToProto`. `codecs.ParseQtyScaled` still returns values up to the `uint64` maximum at scale 18. Migration: keep the scaled quantity at or below 9223372036854775807, which is 9.223372036854775807 at scale 18.
- A decimal order quantity on a market whose base quantity scale is a known 0, such as BONK-USDT, is now encoded as whole units. `Orders.Create`, `Orders.Preview`, `Orders.BatchCreate`, `Orders.Modify`, `Orders.ModifyWith`, `Orders.BatchReplace`, `Triggers.Create`, `codecs.ResolveQtyScaled`, `codecs.ResolveOptionalQtyScaled` and the order and trigger `*ToProto` codecs refused it with "qty requires known quantity scale", though a typed quantity at scale 0 was accepted. A fractional quantity there is refused with "qty supports at most 0 decimal places". A negative scale still means unknown and is refused, and a market missing from the catalog still is.
- **Breaking:** `codecs.FormatQtyScaled` and `codecs.FormatPriceTicks`, and the read decoders that use them, format -9223372036854775808 with one leading minus sign, for example `-9.223372036854775808` at scale 18, instead of `--9.223372036854775808`, or `-0.0000000000000000-9223372036854775808` at scale 36. Every other value formats as before. Migration: none, unless you worked around the doubled sign.
- **Breaking:** `Order.CumQuote`, `Order.QuoteUsed` and `Order.QuoteUnspent` now work at every base quantity scale the catalog accepts, 0 to 36. At base scales 28 to 36 the amount is at 37 to 45 decimals, which the formatter refused, so `CumQuote` returned an error and the others returned not ok. `CumQuote` now returns `*errors.ValidationError` for a base scale below 0 instead of formatting at the wrong scale, and above 36 as before. Migration: pass the pair's scales from the catalog, `catalogs.Manager.BaseQuantityScaleForSymbol` and `QuoteQuantityScaleForSymbol`.
- **Breaking:** `polyester.Config` and `auth.Credentials` no longer expose the private key through any standard printing or encoding path. `encoding/json`, `encoding/xml`, gob and the `log/slog` JSON handler wrote `Config.APIPrivateKey` and the `Credentials` key. A `Credentials` value printed with `fmt` emitted its key bytes, verbs such as `%d` emitted `Config.APIPrivateKey`, and either type printed its key when held in an unexported struct field, which `fmt` reads without calling methods. To close every path, the key now lives where reflection can't reach it:
  - `Config.APIPrivateKey` is now an `auth.Secret` instead of a `string`. The new `auth.NewSecret`, `Secret.Reveal` and `Secret.IsSet` build and read it. `Secret.String`, `Secret.GoString` and `Secret.Format` print `[REDACTED]` for every verb (`""` when the secret is unset), `Secret.MarshalText` writes `"[REDACTED]"` (or `""` when unset), `Secret.UnmarshalText` sets the value, so decoding a JSON `Config` still reads `APIPrivateKey`, and `Secret.GobEncode` refuses with `*errors.AuthError`.
  - The `Credentials.PrivateKey` field is now the `Credentials.PrivateKey()` method, which returns a copy. Build `Credentials` with `LoadCredentials` or the new `auth.NewCredentials`, which validates the key ID and the 64-byte key.
  - `Config` and `Credentials` gain `Format`, `MarshalJSON` and `MarshalXML`, which write the redacted form for values and pointers and in nested fields, slices and maps, and `GobEncode`, which refuses with `*errors.AuthError`. `Config` encoding also redacts URL userinfo in `APIURL` and `WSURL`, as `Config.String` does, and leaves out `HTTPClient`, so a `Config` with a custom client now encodes instead of failing on its function fields.
  - A nil `*Credentials` formats as `<nil>`, and `auth.SignRequest` with an explicit timestamp returns `*errors.AuthError` for credentials without a key instead of panicking. `Config.String`, `Config.GoString`, `Credentials.String` and `Credentials.GoString` print as before.

  Migration: write `APIPrivateKey: auth.NewSecret(hexKey)` in `Config` literals and read it back with `cfg.APIPrivateKey.Reveal()`. Replace `&auth.Credentials{KeyID: id, PrivateKey: key}` with `auth.NewCredentials(id, key)` and `creds.PrivateKey` with `creds.PrivateKey()`. Read the key explicitly where you store it on purpose, instead of marshalling the whole value.
- `Orders.BatchCancel` and `Orders.BatchReplace` (and `codecs.BatchCancelOrdersToProto` and `codecs.BatchReplaceOrdersToProto`) accept up to 50 items, the limit the public contract sets, instead of 20; 51 or more still return `*errors.ValidationError` before sending. `Orders.BatchCreate` keeps its limit of 20. **Breaking:** the limits are read from the contract's validation rules, and `codecs.MaxBatchOrderItems` is replaced by `codecs.MaxBatchCreateItems`, `codecs.MaxBatchCancelItems` and `codecs.MaxBatchReplaceItems`. Migration: size each batch by its operation's limit.
- **Breaking:** A `BatchReplaceAdmissionItem` from `Orders.BatchReplace` (and `decode.BatchReplaceFromProto`) has an empty `ReplacementOrderID` when the server sent no successor, as for an amended or rejected item, instead of `"0"`. `OldOrderID` is likewise empty when the server named no order. Status items from `Orders.GetBatchReplaceStatus` and `Orders.WaitBatchReplace` already left an absent successor empty, as the Python SDK does. Migration: test for `""` instead of `"0"`; a successor ID is still set for a replaced item.
- **Breaking:** `Triggers.Modify` returns `*errors.ValidationError` before sending for a `TriggerPrice` or `ActivationPrice` bound to another market when `symbol` or `symbolID` is given, as trigger creation and order writes already did. It used to send the price's ticks, so a SOL-USDT price could move a BTC-USDT stop. A bound price for a symbol ID the catalogs don't know, or with no catalogs, is refused too, since it can't be checked. Without a symbol or symbol ID the trigger's market isn't known and the price is sent as before. `codecs.ModifyTriggerToProto` takes the market's symbol after `symbolID`; pass `""` when it isn't known. Migration: pass a price for the trigger's market, or an unbound one, and add the symbol argument if you call the codec directly.
- **Breaking:** `models.Order` from `Orders.Get`, `Orders.ListOpen`, `Orders.ListHistory`, `Orders.WaitForOrderTradesComplete` and `Orders.Subscribe` (and `decode.OrderFromProto`, `decode.GetOrderFromProto`, `decode.OrdersListFromOpen`, `decode.OrdersListFromHistory` and `decode.OrdersFromProto`) now has the order's terminal time, origin and MARKET slippage protection, which the server already sent. `TerminalTsNs` is when the order became terminal, in Unix nanoseconds, and is empty while it is open. `Origin` (`models.OrderOrigin`) names its `Scope` (`direct`, `attached_risk`, `standalone_trigger` or `system`) and, for a child of a trigger or attached-risk leg, its `TriggerType`, `TriggerID`, `ParentOrderID` and `ChildSeq`; it is nil when the server sends none. A MARKET order's `MarketMaxSlippage` (a price distance) or `MarketMaxSlippageBps`, and the `MarketClientRefPrice` it was measured from, are nil or zero when the order has none. The new fields mean an unkeyed `models.Order{...}` literal no longer compiles. Migration: name the fields in any `models.Order` literal you build yourself.
- **Breaking:** `Orders.WaitForOrderTradesComplete` returns the final read's `ExecutionsComplete`, which it always reported as false. Its timeout now bounds the whole wait: a page read still in flight when the timeout passes is abandoned, and a read that finishes after it is not returned as a success, so a slow multi-page read times out instead of succeeding after the timeout, and a stalled page returns at the timeout instead of when the HTTP timeout ends it. The timeout error now wraps `context.DeadlineExceeded`. A `next_page_token` that repeats a page token of the same read returns `*errors.ResponseContractError` instead of paging forever. Migration: read `ExecutionsComplete` from the result, pass a timeout long enough to read every execution page, and handle `*errors.ResponseContractError` like any other failed read.
- **Breaking:** A call whose context is cancelled or passes its deadline now returns an error for which `errors.Is(err, context.Canceled)` or `errors.Is(err, context.DeadlineExceeded)` is true, for every API call, including retried reads such as `Triggers.Get`, whether the context ends during a read or between retries. It was a `*errors.TransportError` that only named the cause in its message. It is still a `*errors.TransportError`, which gains an `Err` field holding the context error and an `Unwrap` method. Migration: build `errors.TransportError` with keyed fields, and use `errors.Is(err, context.Canceled)` or `errors.Is(err, context.DeadlineExceeded)` instead of matching the message.

## 0.10.0

Git tag: `v0.10.0`. Released 2026-11-07. Built on public contract `748066a`.

- **Breaking:** A zipped-asset supply at a quantity scale of 0 now reads as whole units. `SubscribeZippedAssetSupply` reported a BONK supply of 3582802 as `0.000000000003582802`, because `codecs.FormatLedgerU128` treated scale 0 as 18. It now formats at the scale you pass, including 0, and returns `*errors.ValidationError` for a negative scale instead of using 18. `codecs.StrToU128Proto` encodes at scale 0 too and returns zero for a negative scale. A zipped asset that isn't in the hydrated catalog is still refused. Migration: pass `codecs.LedgerScale` to either function for 18-decimal balances.
- **Breaking:** `GuardApproval` now has `NonceSpace` and `DeadlineUnix` from `SignProtectedAction` and `BatchSignProtectedActions`, as exact `uint64` values, and `RotateGuardSignerWalletResult` has the rotation's `Approval`, nil when the API returns none. Submit them with the signature. Added `decode.GuardApprovalFromProto`. Migration: use keyed fields in `models.GuardApproval{...}` and `RotateGuardSignerWalletResult{...}` literals.
- **Breaking:** `Orderbook.Get` reports the snapshot depth it requested as `Depth`. A depth between snapshot depths requests the smallest one covering it, as before, but reported the depth you passed: 11 reported 11 for a 20-level snapshot. 0 requests and reports 50 instead of requesting 5 and reporting 0. A depth below 0 or above 1000 is now an `*errors.ValidationError` before any request, instead of requesting 5 or 1000 and reporting the depth passed. Migration: read `Depth` for the depth you got, pass a depth from 0 to 1000, and trim the levels yourself if you need fewer. `codecs.OrderbookSnapshotDepth` gives the depth requested for a depth.
- **Breaking:** `orderbook.ApplyDelta` refuses a delta with a positive quantity at price zero like any other malformed delta: it changes neither side, keeps the sequence and asks for a snapshot refresh, instead of applying it, advancing the sequence and leaving a book `BuildOrderbookData` can't render. A managed subscription therefore rebuilds its book from a fresh snapshot (`OnSequenceGap`, then `OnSnapshotRefresh`) on such a delta and stays open, instead of closing with an invalid-price error. Migration: none needed; code that resubscribed when `Updates()` closed on that error no longer sees it.
- `orderbook.BuildOrderbookData` returns `*errors.ValidationError` for a negative depth instead of panicking. Depth 0 still renders no levels.
- **Breaking:** `LedgerTransfer` from `Transfers.List` and `Transfers.Subscribe` now has `BalanceAfter` and `LinkID`, which the server already sent, so an unkeyed `models.LedgerTransfer{...}` literal no longer compiles. `BalanceAfter` is the account bucket's balance after the transfer, an exact integer at the 18-decimal ledger scale like `Amount`; it is nil when the server sends none, and a zero balance is `"0"`. `LinkID` is the decimal correlation ID shared by related legs, empty when there is none. Migration: name the fields in any `LedgerTransfer` literal you build yourself, or add the two new values.
- Private realtime subscriptions (balances, transfers, trades, triggers, trigger events, lifecycle flows, sub-accounts, API keys, API and sub-account policies, and address books) now renew their subscription and connection tokens on the same connection before they expire. The server used to end each one about once a token lifetime with code 2501, or close the connection, and it reconnected, reporting the error and counting a resubscribe every time. A renewal that the token API or the server refuses is reported through the error callback; the server then ends the subscription and it resubscribes as before. `Orders.Subscribe` already renewed its tokens and is unchanged.
- **Breaking:** `Orders.CancelAllAfter` and `codecs.CancelAllAfterToProto` return `*errors.ValidationError` before sending for a timeout below 0 or above 4294967295. It used to be wrapped into the 32-bit field: 4294967296 sent 0, which disables the dead-man switch, and 4294967306 armed it for 10 seconds. Migration: pass 0 to disable the switch or a timeout the server accepts (10 to 120 seconds).
- **Breaking:** `GuardSigner.SignProtectedAction` and `GuardSigner.BatchSignProtectedActions` return `*errors.ValidationError` before sending for an external chain ID below 0 or above 4294967295, which used to be wrapped to another chain (4294967297 sent 1). A batch action's `external_polychain_chain_id` must be a whole number: a fraction is refused instead of truncated, and a string, bool or other non-number is refused instead of being dropped and sent as no chain. Unsigned integers and `json.Number`, which were dropped the same way, are now sent. `codecs.ProtectedActionArgsToProto` and `codecs.BatchProtectedActionItemToProto` now also return an error. Migration: pass the chain ID as a whole number from 0 to 4294967295, and handle the new error if you call those codecs directly.
- **Breaking:** `StopAllTrading` returns `*errors.ResponseContractError` when `ListTriggers` answers with any page token it already sent in the same call, instead of paging forever when the tokens cycle (A, B, A). Only a token repeating the one just sent was caught before. Nothing is canceled then, and the error says so. Migration: on this error, cancel your triggers and orders another way, for example `Triggers.Cancel` for each trigger you know and `Orders.CancelAll`.

## 0.9.0

Git tag: `v0.9.0`. Released 2026-11-07. Built on public contract `748066a`.

- `Orderbook.CreateSubscription` at a depth the server doesn't publish no longer goes silent after its first view. Depths 5, 10, 25 and 100, for example, were accepted by the server and got no updates, with no error. Any depth from 1 to 500 now subscribes to the smallest published depth that covers it (1, 20, 50, 200 or 500), takes its snapshot there, and trims each view to the depth you asked for. Sequence-gap, availability and bucket behaviour are unchanged, and a view can update when only levels outside it changed.
- **Breaking:** `Orderbook.CreateSubscription` returns `*errors.ValidationError` before connecting for a depth above 500, which used to be clamped to 500 and report that depth, and for a negative depth, which used to mean 50. A depth of 0 still means 50. Migration: pass a depth from 1 to 500, or read a snapshot of up to 1000 levels with `Orderbook.Get`, which is unchanged.
- **Breaking:** `Orderbook.SubscribeDeltas` returns `*errors.ValidationError` before connecting for any depth other than 1, 20, 50, 200 or 500, instead of subscribing to a stream that sends nothing or clamping to 1 or 500. Raw deltas can't be trimmed. Migration: pass a published depth, or use `Orderbook.CreateSubscription` for a book at any depth up to 500.
- Added `codecs.OrderbookStreamDepths`, `codecs.OrderbookStreamTier` and `codecs.RequirePublishedStreamDepth`, the published stream depths and the checks the client uses.

## 0.8.0

Git tag: `v0.8.0`. Released 2026-11-07. Built on public contract `748066a`.

**Known issue:** a managed order book subscribed at a depth other than 1, 20, 50,
200 or 500 can stop updating with no error. Use one of those depths until the
next release.

- **Breaking:** A new order stream subscription the server never confirms now ends after 30 seconds with an error, instead of waiting forever while heartbeats keep the connection alive. A resubscribe on a confirmed stream that gets no answer reports `OrderStreamError` and subscribes again with backoff.
- **Breaking:** A realtime subscription the server ends no longer stays open and silent. `Messages()` delivers what is queued and closes, and `Err()` and the error callback report the new `*errors.SubscriptionEndedError` with the channel, the server's code and text, and `Reason`. Migration: handle a closed channel; subscribe again if you still need the stream. Server unsubscribe codes from 2500 ask for a new subscribe and reconnect as before.
- A sub-account's balances, transfers, trades, triggers and lifecycle subscriptions now resume after a server unsubscribe while `ListSubaccounts` lists the sub-account with a role, using a fresh token and reporting the gap through `Resubscribes()`. They stop with the reason after a `removed` update on this client, when not listed, when the list or token is refused, or after 6 attempts. List reads and attempts are paced across the client.
- **Breaking:** Managed order books no longer emit a default, empty-looking view before the fetched snapshot, on `Updates()` or `OnEvent`, and `SetBucket` re-renders only once a snapshot is applied. Migration: remove any code that skips the first view.
- Managed order books request an unavailable snapshot again, after 1 s doubling to 8 s, until it is available or the subscription stops. Repeated unavailable answers add no view. A market that stays unavailable keeps being requested once every 8 s. A failed request ends the subscription like a failed sequence-gap refresh.
- **Breaking:** An HTTP error with no recognised code no longer reports a known failure, because a gateway or an edge can send it too. No code, an empty code, the placeholder `ERROR_CODE_UNSPECIFIED` or `AUTH_UNSPECIFIED`, a service detail with no code or with a code this SDK can't name, unless a recognised refusal Connect code stands behind it, an unrecognised code, Connect `unknown` or `data_loss`, HTML, text or an empty body now return `ServerError` with outcome unknown and not retryable at every status except 401 and 403, which stay `AuthError`. That includes 400, 404 with a JSON body, 405, 409, 412, 422, 500, 501 and 520, which returned `APIError` or `NotImplementedError`. Trigger reads still retry 429, 502, 503 and 504; retry other reads yourself if you want to. Creates are sent once. A bare 404 stays `RouteNotFoundError`, now with outcome unknown. A well-formed unrecognised code at 429 stays a known `APIError`. REST `INTERNAL_ERROR`, `SERVER_ERROR` and `GATEWAY_MISCONFIGURED` at 500 now return `ServerError`, REST codes at 413 keep their code instead of `unknown`, and REST `TRIGGER_NOT_FOUND`, `NOT_FOUND` and other contract codes at 404 return `APIError` instead of `RouteNotFoundError`. A gzip-compressed REST problem now keeps its code. Migration: read back after these errors instead of treating them as refusals, and branch on `Code` only for codes the contract defines; a validation code newer than this SDK reads outcome unknown until you upgrade, which errs toward reading back.
- Trigger reads return a definite REST refusal (`application/problem+json` at 400, 401, 403, 409, 412, 422 or 429, and `ACCOUNT_ROUTING_INVALID` at 503) on the first request with its typed reason, as they already did for a Connect error, instead of retrying it for two seconds. Real unavailability keeps its bounded retry.
- A zero or negative attached trailing stop maximum on modify or batch replace now names what's wrong: `ValidationError` with `ERROR_CODE_TRIGGER_BUY_REQUIRES_LIMIT`, rule `buy_required`, and the `new_attached_risk.trailing_stop.limit_price_ticks` field. It was already refused before sending.
- **Breaking:** A ledger refusal carrying `ledger.read.v1.ErrorDetail` now reports `APIError` with its specific code, such as `ERROR_CODE_INVALID_SUBACCOUNT_ID` (11), instead of a `ValidationError` with code `invalid_argument`. Migrate by branching on `APIError.Code`.
- Shared signing vectors now cover escaped URL paths. Go already signed the decoded path, as the server verifies it.
- **Breaking:** A live order-book delta with a zero or missing price and positive quantity now closes the managed stream with an invalid-price error at every bucket and depth, instead of showing it as a level; `BucketSide` and `BuildOrderbookData` refuse it too. Any other book render failure also ends the stream instead of leaving it open. Migration: when `Updates()` closes, check `Err()` and resubscribe.

- **Breaking:** Bare REST `UNAVAILABLE` cannot establish an orders-service refusal: report outcome unknown and non-retryable. Only a typed `orders.v1.ErrorDetail` carrying `UNAVAILABLE` establishes a known, retryable refusal. Migration: read back after an unknown outcome; retry reads explicitly.
- **Breaking:** The additional retry-later resource codes `CLIENT_ORDER_IDS_EXHAUSTED`, `CLIENT_ORDER_ID_CAPACITY_REACHED`, `MARKET_LADDER_LIMIT_EXCEEDED`, `MARKET_TRIGGER_LIMIT_EXCEEDED`, `MARKET_TWAP_LIMIT_EXCEEDED`, `NEW_ACCOUNT_CAPACITY_REACHED`, `REQUEST_RECEIPTS_EXHAUSTED`, `RESERVATION_CAPACITY_REACHED`, `TRIGGER_CAPACITY_REACHED`, `TRIGGER_CONTROL_SHARE_EXHAUSTED`, `TWAP_CAPACITY_REACHED` report outcome known and retryable, preserving any server retry delay.
- **Breaking:** Typed orders refusals retain provenance in `ServerError.AdmissionRefused`; generic REST `UNAVAILABLE` cannot establish that provenance.
- **Breaking:** Unknown mutation outcomes are never retryable, including bare Connect unavailable/internal/deadline failures, gateway 502/503 responses, bare or HTML 429 responses and client timeouts. Migration: read back after an unknown outcome; retry reads explicitly. Bounded trigger reads, status waits and snapshot recovery use read context; creates are never automatically resent.
- **Breaking:** Structured order-service admission refusals now report outcome known while remaining retryable. This corrects the previous outcome-unknown result for typed `orders.v1.ErrorDetail` codes `UNAVAILABLE`, `FEE_SCHEDULE_UNAVAILABLE`, `POLICY_UNAVAILABLE`, `RATE_LIMIT_POLICY_UNAVAILABLE` and other documented temporary admission refusals. Specific code-only Connect messages now preserve their refusal code. `ACCOUNT_ROUTING_INVALID` is known and non-retryable. Unrecognized dependency failures remain ambiguous.
- **Breaking:** REST validation refusals retain their service codes with outcome known and no retry, while timestamp skew retains its explicit safe-retry guidance. Account repairing retains its explicit safe-retry guidance. REST `RATE_LIMIT_EXCEEDED` and `PRICE_LEVELS_EXHAUSTED` are known admission refusals with retry delays preserved. All REST, typed Connect and message-only resource codes now follow their contract retry guidance; outcome-unknown codes and details take precedence over standalone backoff details.
- **Breaking:** `APIError.Problem` now also retains REST refusal details at HTTP 400, 409, 412 and 422.
- **Breaking:** Batch-create items now refuse any subaccount override instead of silently using the batch scope. Migration: remove each item's `SubAccountID` and pass the batch-level `subAccountID`.
- **Breaking:** Order create and preview now refuse an account selector combined with any explicit subaccount override. Migration: pass either `account` or the request's `SubAccountID`.
- **Breaking:** Batch guard signing now refuses a subaccount or account key on an action instead of silently ignoring it. Migration: remove the key and pass the batch-level `subAccountID`.

- **Breaking:** An explicit empty subaccount override now omits the subaccount field instead of silently selecting the configured default, including on writes. Omitted overrides and `account="main"` keep their existing routing.
- **Breaking:** Empty overrides now fail where a subaccount is required, or when combined with an account selector. Migration: pass a nonempty subaccount, or omit the override to use the configured default.
- **Breaking:** Realtime protobuf subscriptions now refuse a closed client with `realtime.ErrClientClosed`. Migration: create a new realtime client to subscribe after `Close`.

- **Breaking:** A plain REST gateway 504 is now non-retryable with mutation outcome unknown. Reads may retry a 504 explicitly; writes read back first. A client order ID enables reconciliation, not exact replay of a single create. Automatic read retries remain operation-specific; creates are never automatically resent.
- **Breaking:** `OVERLOADED` and `SYSTEM_RATE_LIMITED` now return `RateLimitError` instead of API/server errors on Connect and REST. They are retryable admission refusals with no unknown mutation outcome. The new `Code` field retains the service code; known order codes use the `ERROR_CODE_` prefix. Use keyed struct literals when constructing rate-limit errors. `ORDER_SEQUENCE_EXHAUSTED` remains a permanent refusal, including REST 429 and Connect responses without typed details. Supplied retry delays are preserved.
- **Breaking:** A Connect message naming an `*_OUTCOME_UNKNOWN` code now classifies as non-retryable with outcome unknown, including codes introduced by a newer server. Known order names are normalized to `ERROR_CODE_…`; future names remain as sent.
- **Breaking:** `APIError` adds `Problem` on the REST outcome-unknown and `ORDER_SEQUENCE_EXHAUSTED` paths, retaining code, actual HTTP status, canonical type and detail. Use keyed struct literals when constructing errors.

## 0.7.1

Git tag: `v0.7.1`. Released 2026-11-06. Built on public contract `748066a`.

- Managed market overview streams finish after terminal failure, report the error, and let callers drain queued rows. Closing after consumer queue overflow no longer blocks.
- Order books detect gaps after a sequence-zero snapshot and refetch before applying later deltas.
- A full managed order book queue shuts down cleanly, preserves its error, and releases the connection.
- Unknown market selectors report that catalog hydration is needed instead of a false name/ID conflict.
- Realtime subscriptions preserve publications sent alongside or before the subscribe reply.
- Managed order books finish their update stream after terminal failure and reject conflicting market name and ID selectors before connecting.

## 0.7.0

Git tag: `v0.7.0`. Released 2026-11-06. Built on public contract `748066a`.

- **Breaking:** trigger creation also refuses invalid `tif` and `order_type`
  values on paths that never read them, and order writes refuse an empty symbol
  sent with a valid symbol ID. These calls were accepted before.
- **Breaking:** Batch create now checks each quantity against its destination pair's catalog
  scale. A mismatched explicit scale raises an error before the batch is sent.
  Create, batch create, modify and batch replace also refuse quantities tagged
  for another pair and conflicting item-level symbols and IDs before sending.
  Migration: use the destination pair's symbol, ID and catalog scale.

- Batch create resolves base and quote-budget scales separately for each item.
- Create results carry the catalog scale for resolved quantities.

- Fixed API-key request signing for repeated query parameters by sorting decoded values as well as keys before encoding. Requests such as `a=2&a=1` now match the server.
- **Breaking:** Trigger creation now rejects options that do not apply to the selected strategy or child execution before sending. Migration: remove incompatible options, or select the trigger type and execution that support them (use `trailing_stop` for trailing distances and activation price).

- Internal transfers now preserve a valid account or subaccount recipient
  when an optional destination address is empty or whitespace.
- **Breaking:** transfer history now rejects negative `since` timestamps with a validation
  error before sending, instead of wrapping them into a large unsigned value.
  Zero and positive values remain inclusive UTC epoch microsecond bounds.
- **Breaking:** `CreateToFunding` and `CreateToExternalChain` now return a
  `ValidationError` if `account` or `subAccountID` is supplied, including
  `"main"`, `"active"` or a selector matching the API key's scope. These
  arguments previously had no effect. Pass `nil` for both selectors and use
  an API key bound to the intended source. Configured SDK defaults do not
  route API-key withdrawals; wallet withdrawals still support account
  selection and configured defaults.
- **Breaking:** trailing-stop creation now rejects competing ticks and basis-point
  trailing distances instead of silently keeping only ticks.
- **Breaking:** transfer history now returns the opaque `NextPageToken` string
  instead of an integer `NextCursor`. Pass it as the optional final argument to
  `Transfers.List` with the same scope, filters and direction. An empty string
  ends pagination. Existing calls without a token still read the first page.
- Preserve market-overview and row-candle continuation tokens. Pass the returned
  token as the optional final argument to `MarketOverview.List`, `GetCandles`,
  or `GetCandlesWithReference` to read the next page.
- Heatmap history now accepts its opaque token as the optional final `Get`
  argument. Continuations omit a new default time window and keep any supplied
  original range bounds unchanged.
- Equity history now exposes `BTCPricesQ`: the returned BTC-USDT closes as
  integers scaled by 1e9, in timestamp order. An omitted series is an empty slice.

## 0.6.2

Git tag: `v0.6.2`. Released 2026-11-06.

- REST errors that say an order or trigger outcome is unknown now use the
  same non-retryable API error as Connect. Read the order or trigger back
  before retrying: the first request may still apply. Codes added by a newer
  server are treated the same way. A gateway timeout
  without a recognised outcome code remains a retryable transport/server
  failure with an unknown outcome; reconcile before retrying a mutation.

## 0.6.1

Git tag: `v0.6.1`. Released 2026-11-06.

- Service-unavailable responses now raise retryable server errors on Connect
  for orders, withdrawals and internal transfers, matching REST. This includes
  policy and fee lookups, backlogs, operations in progress and temporarily
  unavailable dependencies. Policy denials and permanent precondition failures
  remain non-retryable. Rate-limit errors keep their retry delay, and unknown
  mutation outcomes remain non-retryable even on an unavailable response.
- Keep the service code and status on Connect server errors, and preserve
  the problem code and HTTP status on REST 502 and 503 server errors.

## 0.6.0

Git tag: `v0.6.0`. Released 2026-11-05.

- **Breaking:** a trailing stop created with an activation price already
  reached now ends `FAILED` with
  `TRIGGER_FAILURE_REASON_TRIGGER_WOULD_TRIGGER_IMMEDIATELY` (244).
  `ModifyTrigger` and order modifies that would make a trailing stop act at
  once are refused with `ERROR_CODE_TRIGGER_WOULD_TRIGGER_IMMEDIATELY` (302),
  leaving the existing state unchanged. To start trailing from the market,
  clear the activation price with a present zero.
- **Breaking:** remove the public holds-list method, hold models and decoders.
  Read aggregate held trading funds from each asset's `reserved` balance in
  the balances API. Individual hold IDs and expiry times have no replacement.
- **Breaking:** listing events for an unreadable trigger now returns the same
  not-found error as reading that trigger, instead of an empty history.
  Events reads retry a catching-up `UNAVAILABLE` with the trigger-read policy.
- Expose reserve fee rates, the next scheduled change and the full fee
  breakdown. Omitted optional components remain absent and omitted account
  discounts decode as an empty list.
- Preserve REST problem status, code, detail and canonical type on realtime
  authorization and early gateway authentication errors.
- Pin execution-history continuation pages to the returned exchange order ID
  so reuse of a client order ID cannot change generations between pages.

**What a trading bot gains.** A bot can read the product changelog of the
environment it trades on, with no API key, and find the breaking or
action-required changes published since it last looked. It can wait for a
batch replace to settle and see which items the market refused after
admission, read a trigger back right after changing it, and tell a market halt
from other cancels. It can see why a trigger's fire placed no order, read an
order's exact average price, quote used and fees, tell why an API key was
refused on either transport, and stop all automated trading in one call.

**Breaking changes**
- A sub-account owner-role refusal is `*errors.SubaccountOwnerRoleLockedError`
  rather than `*errors.APIError`. `errors.As` with `*errors.APIError` and
  `errors.AuthErrorCode` still match it; a direct type assertion does not.
- A batch replace status item with no successor reports an empty
  `ReplacementOrderID` instead of `"0"`.
- A session-only method refusing an API key (`AUTH_SESSION_KIND_NOT_ALLOWED`)
  is `*errors.SessionRequiredError` rather than `*errors.APIError`.
  `errors.As` with `*errors.APIError` and `errors.AuthErrorCode` still match
  it; a direct type assertion does not.
- `errors.AuthErrorCode` returns only auth reasons (`AUTH_*`). It used to
  return any `*errors.APIError` code, such as an order `ERROR_CODE_*` or a
  Connect `not_found`; read `APIError.Code` for those.
- `SubAccounts.Get` takes `includeBalances` again, after `includePolicy`:
  `Get(ctx, account, subAccountID, includeAPIKeys, includeMembers,
  includeInvites, includePolicy, includeBalances, invitesDirection)`. Pass
  `false` to keep the previous behavior.

### Added
- `client.Changelog` wraps the public `changelog.v1.ChangelogService`.
  `ListPosts`, `GetPost` and `ListChanges` return typed
  `models.ChangelogPostSummary`, `ChangelogPost`, `ChangelogSection`,
  `ChangelogChange` and `ChangelogChangeEntry` values. `Posts` and `Changes`
  iterate over every page, and `BreakingChangesSince` and
  `ActionRequiredSince` collect the matching changes published since a time.
  Categories, audiences and surfaces are lowercase labels, and a value newer
  than the SDK reads as `unknown(<n>)`. Change IDs are base58. An unknown slug
  fails with `*errors.APIError` code `not_found`.
- `Orders.WaitBatchReplace` polls `GetBatchReplaceStatus` until no item is
  `admitted`, through the not found a new batch's status reads for a few
  seconds after admission, and stops at the 60-second status window, even
  with a status read in flight, with `*errors.BatchReplaceWindowExpiredError`
  and the last status: reconcile the orders with order reads then. A canceled
  or expired `ctx` returns `ctx.Err()`.
- `BatchReplaceStatusItem.ErrorCode` names an item's refusal as the
  `orders.v1` error code a single modify returns (`ERROR_CODE_POST_ONLY_CROSS`
  for `Code` `POST_ONLY_CROSS`); a code newer than the SDK is kept as sent.
- Market halts: orders the halt cancels read `CancelReason` `market_halted`
  (`CANCEL_REASON_MARKET_HALTED`, 17) on order reads and the order stream, and
  triggers it ends read `CancelReason` `market_halted`
  (`TRIGGER_CANCEL_REASON_MARKET_HALTED`, 13) on trigger reads, events and
  streams. A trigger refused after a halt reads `FailureReason`
  `unknown(212)` until the server reports `market_halted`.
- `*errors.SubaccountOwnerRoleLockedError` and
  `errors.IsSubaccountOwnerRoleLocked`: a sub-account member call that targets
  the owner (`AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED`, Connect
  `failed_precondition` or REST 412). It unwraps to `*errors.APIError`, so
  `errors.AuthErrorCode` still reads the code; a direct type assertion to
  `*errors.APIError` no longer matches it.

- Trigger events `fire_refused` (`EVENT_FIRE_REFUSED`): a fire whose order
  was refused while the trigger stayed live, for the market's state, a
  position below the market's minimums, a rate limit or an account repair.
  It is not terminal. `TriggerEvent.FireRefusalReason` says why, with the
  `FailureReason` labels, which gain `account_repairing`. Decoded on
  `Triggers.ListEvents` and the trigger event stream; `ListEvents` filters on
  `fire_refused`.
- `errors.AuthErrorCode` reads the auth reason of either transport: the
  Connect `auth.v1.AuthErrorDetail` code, or a REST problem `code` with the
  `AUTH_` prefix added (`UNAUTHENTICATED` reads
  `AUTH_AUTHENTICATION_REQUIRED`). Constants for the new API-key refusals:
  `errors.AuthCodeSignatureInvalid`, `AuthCodeTimestampSkew`,
  `AuthCodeMissingHeaders`, `AuthCodeKeyExpired`, `AuthCodeKeyDisabled`,
  `AuthCodeIPNotAllowed`, `AuthCodeAPIKeyNotAccepted`, plus
  `AuthCodeAuthenticationRequired` and `AuthCodeSessionKindNotAllowed`.
- `*errors.SessionRequiredError` and `errors.IsSessionRequired`: an API key on
  a method that accepts only a session token, on every service (Connect
  `permission_denied` with `AUTH_SESSION_KIND_NOT_ALLOWED`, REST 403).
- `errors.FieldViolation` and `errors.Violations(err)`: the fields an order
  refusal names, from the order `ErrorDetail`, from request validation
  (`buf.validate.Violations`, which the SDK used to drop) or from a REST
  problem's `violations`. `ValidationError.Violations` holds them;
  `Metadata` keeps its `violation.N.*` entries. `models.OrderFieldViolation`
  is now an alias of `errors.FieldViolation`.
- `Order.AvgPriceE18` and `Order.AvgPrice()`: the exact average execution
  price at 18 decimals. `Order.CumNotional` and `Order.CumQuote(baseScale)`:
  the exact quote amount of the fills. `Order.FeeTotal`
  (`models.OrderFeeTotal`): the fills' net fee, set on history and
  single-order reads with execution history. `Order.QuoteUsed(baseScale)` and
  `Order.QuoteUnspent(baseScale, quoteScale)` say how much of a quote-amount
  buy was used, fee included, once the fee total covers every fill.
- `errors.IsOrderAlreadyTerminal`: a cancel of an order that has already
  ended, which a repeat of the cancel that ended it now also answers.
- `client.StopAllTrading` (`services.StopAllTrading`): cancels every live
  trigger in scope (created, armed, running, paused, every page), then all
  open orders. `Orders.CancelAll` and the dead-man switch cancel open orders
  only, never triggers.
- Changelog surfaces `market_data`, `quickswap` and `rewards`
  (`models.ChangelogSurfaceMarketData`, `ChangelogSurfaceQuickSwap`,
  `ChangelogSurfaceRewards`), and `ChangelogSection.Surface`, the product area
  a section covers. `ListChanges` refuses more than 10 surfaces before
  sending.

- `SubAccounts.Get` with `includeBalances` returns the sub-account's current
  balances in `GetSubaccountResult.Balances`, a `models.BalancesList` as
  `Balances.List` returns. It is nil when not asked for. It needs the
  `READ_BALANCES` action on the sub-account or fails with PermissionDenied;
  when balances cannot be read right now it fails with the retryable
  `AUTH_TEMPORARILY_UNAVAILABLE`.
- `CreateOrderRequest.SelfTradePreventionMode` (`expire_maker`, the server
  default, `expire_taker` or `expire_both`) on create, preview and batch
  create. It applies to the order's attached take-profit, stop-loss and
  trailing stop orders as well. An unknown mode is refused before sending.
  `Order.SelfTradePreventionMode` reads it back, and a mode newer than this
  SDK reads as `unknown(<n>)`.
- Attached exits never trade with the order they protect: an order whose
  exit fired while it still rested ends canceled with `CancelReason`
  `attached_exit_fired`, and an exit order that self-trade prevention expired
  against another of the account's resting orders is a `fire_refused` trigger
  event with `FireRefusalReason` `self_trade_prevented`, whose `ChildSeq`
  names that order; the leg stays armed. `RejectReason` and order error codes
  read the new `SELF_TRADE_PREVENTED` code, which no request is answered with.

### Changed
- A trigger the halted market refuses reads `FailureReason` (and a refused
  fire `FireRefusalReason`) `market_halted` on servers on this contract; it
  used to read `unknown(212)`. The SDK has no special case for 212, and any
  unknown reason still reads as `unknown(<n>)`.
- Docs: a buy sized by a quote amount takes attached risk (no SDK guard ever
  refused it). `Order.CumNotional`, `CumQuote` and `QuoteUsed` are exact when
  the pair's base quantity scale is at most 9, and can exceed the settled
  quote by less than 1e-18 per fill above that.
- A Connect API-key refusal with `AUTH_TIMESTAMP_SKEW` is the retryable
  `*errors.TimestampSkewError`, as the gateway's `TIMESTAMP_SKEW` answer is.
  The other API-key refusals (`AUTH_SIGNATURE_INVALID`, `AUTH_IP_NOT_ALLOWED`,
  ...) are `*errors.AuthError` with that code and the server's message, as
  the bare `unauthenticated` they replace was; they are not retryable.
- A REST problem body answering a Connect call keeps its `detail` and `code`:
  API-key refusals and `PERMISSION_DENIED` read as `*errors.AuthError`
  (for example "api key policy does not allow READ_AUTO_WITHDRAW", which names
  the policy action to grant), and a problem with `violations` as
  `*errors.ValidationError`. They used to read as a bare HTTP status.
- The attached trailing stop's `LimitPrice` checks on create, preview and
  batch create refuse with the server's code, rule ID and field
  (`order.attached_risk.trailing_stop.limit_price_ticks`):
  `ERROR_CODE_BAD_PRICE` (`sell_unset`, `buy_at_least_activation`) and
  `ERROR_CODE_TRIGGER_BUY_REQUIRES_LIMIT` (`buy_required`). A zero
  `LimitPrice` is no maximum: a sell parent is refused with
  `buy_required` (it was a generic "must be positive" error with no code).
  Other messages are unchanged.
- A trigger event type newer than this SDK reads as `unknown(<n>)` instead of
  its bare number.
- Docs: `Orders.CancelAll` and `CancelAllAfter` state that triggers stay
  live; modify and batch replace inputs state that the server applies the
  trailing stop's side rule.
- `Triggers.Get` and `Triggers.List` retry `UNAVAILABLE` (HTTP 503) with the
  same request, from 25 ms doubling up to 500 ms, for about 2 s. Trigger reads
  answer it until they reflect an answered modify, pause or resume.

### Fixed
- `Orders.GetBatchReplaceStatus` no longer fails with
  `*errors.ResponseContractError` when an item was admitted and then refused
  when applied. Servers now report such an item `rejected` with its code while
  the admission counts still count it as accepted.

## 0.5.1

Git tag: `v0.5.1`. Released 2026-11-02.

**What a trading bot gains.** A sell order can carry an attached trailing
stop again. On a sell order the trailing stop buys back, and the server now
requires its maximum buy-back price; `TrailingStopInput.LimitPrice` sends it.

**Breaking changes**
- None in the SDK API.

### Fixed
- `TrailingStopInput.LimitPrice` sets an attached trailing stop's buy-back
  limit price, the most its exit pays per unit, which also sizes the funds it
  reserves. A sell order's attached trailing stop requires it, and a buy
  order's must leave it nil; creates, previews and batch creates check this
  before sending. It must be positive and at least `ActivationPrice` when both
  are set. Modify and batch replace send it as given, and the server applies
  the side rule there. Order reads do not return it yet.

## 0.5.0

Git tag: `v0.5.0`. Released 2026-11-02.

**What a trading bot gains.** An attached take-profit, stop-loss or trailing
stop now says when it starts protecting the position: its trigger events
include `armed`, once, when the parent order's first fill makes the leg live.
A bot can wait for it instead of polling the leg's status. Three new error
codes are final refusals: the request did not apply, so do not resend it
unchanged.

An environment reports `armed` events and the new codes once it runs this
contract: devnet first, then testnet after its next upgrade.

**Breaking changes**
- None in the SDK API.

### Added
- Trigger events decode the new `armed` event type: an attached take-profit,
  stop-loss or trailing-stop leg reports it once, when its parent order's
  first fill makes the leg live (or when a leg paused before that fill
  resumes). It is not terminal. `Triggers.ListEvents` accepts `"armed"` as its
  event type filter. Attached trailing stops still report `activated` when
  they start tracking.
- Error codes `AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED` (the sub-account's owner
  cannot be invited, re-roled or removed),
  `ERROR_CODE_TRIGGER_SYMBOL_MISMATCH` (modify, pause or resume named a listed
  pair other than the trigger's own; name the trigger's pair or omit it) and
  the balance reads' `ERROR_CODE_INVALID_SUBACCOUNT_ID` (a zero or malformed
  sub-account ID).
- Generated messages add `Subaccount.removed` (real-time sub-account updates
  only: the caller lost access, so drop the sub-account),
  `TrailingStopPolicy.limit_price_ticks`, `Checkpoint.opening_value_usd`,
  `ListMyRewardAwardsResponse.awards_may_change` and the Swap reason
  `MAX_ORDER_QUANTITY`, and the trade WebSocket protobufs follow binary schema
  34.

### Server behaviour on this contract
- Batch replace amends an order in place, keeping its order ID and queue
  priority, when an item keeps the price, does not raise the total quantity
  and keeps the client order ID. Otherwise it replaces the order under a new
  order ID.
- An attached trailing stop on a selling order buys back, and the server now
  requires its maximum buy-back price. This SDK's `AttachedRisk.TrailingStop`
  does not send that price yet, so place a selling order with only take-profit
  and stop-loss legs until a later release adds it. A buying order's attached
  trailing stop is unchanged.
- Ladder outcomes: a ladder is completed once no child order remains open and
  at least one reached the book; it is failed only when every child order was
  rejected before reaching the book.

## 0.4.0

Git tag: `v0.4.0`. Released 2026-11-02.

**What a trading bot gains.** An order now says why it ended, in the same
words everywhere. A rejected order's `RejectReason` is the error code the
order write API returns for the same refusal, and a canceled order's
`CancelReason` says why it was canceled, on order reads and on the private
order stream alike. Before, the numeric reason used the matching engine's own
numbering, which clashed with the API's error codes. `OrderbookData.Available`
tells a bot when a book's levels are unknown rather than empty, so it does not
quote into a book that is being rebuilt. `Trigger.Version` picks out the
newest copy of a trigger, and nine new error codes are final refusals: the
request did not apply, so do not resend it unchanged.

This release works on devnet now, and on testnet after testnet's next upgrade.

**Reason labels in this SDK.** `RejectReason` is the `orders.v1.ErrorCode`
name without its `ERROR_CODE_` prefix, for example `POST_ONLY_CROSS`.
`CancelReason` is the `orders.v1.CancelReason` name without its
`CANCEL_REASON_` prefix, in lowercase, for example `user_request`. The Python
and Rust SDKs format these labels differently.

**Breaking changes**
- `Client.Whiteboard`, `Client.Polychart` and `Client.Layout` are removed,
  with `codecs.BoardAudienceFromLabel` and `codecs.BoardRoleFromLabel`.
- The generated `orders.v1.Order` loses `TerminalReasonCode` and
  `TerminalReason`, and the generated `privatews.v1.OrderEvent` loses
  `ReasonCode`. Read `RejectReason` and `CancelReason` instead.
- `models.OrderEvent.ReasonCode` is replaced by `RejectReason` and
  `CancelReason`.
- `SubAccounts.Get` no longer takes `includeBalances`.

### Breaking
- `Client.Whiteboard`, `Client.Polychart` and `Client.Layout` are removed,
  because those APIs are no longer part of the public API.
- `codecs.BoardAudienceFromLabel` and `codecs.BoardRoleFromLabel` are
  removed with them.
- The generated `orders.v1.Order` loses `TerminalReasonCode` and
  `TerminalReason`, and the generated `privatews.v1.OrderEvent` loses
  `ReasonCode`. Their numbers used the matching engine's own numbering, which
  collides with `ErrorCode`. Read the typed `RejectReason`
  (`orders.v1.ErrorCode`) and `CancelReason` (`orders.v1.CancelReason`)
  instead, or the decoded labels below.
- `models.OrderEvent.ReasonCode` is replaced by `RejectReason` and
  `CancelReason`. A rejected order's event names its refusal as the order
  write API does (for example `POST_ONLY_CROSS`). A canceled order's event
  names why it ended (for example `user_request`), and so does the replaced
  event of the order a replace ended (`replaced`). Both are empty otherwise.
- `SubAccounts.Get` no longer takes `includeBalances`: a sub-account read no
  longer returns balances. Read them with `Balances.List` and the sub-account ID.

### Added
- `Order.RejectReason` and `Order.CancelReason`, on order reads and on the
  private order stream's snapshot and unit orders. A rejected order's
  `RejectReason` is the `ErrorCode` name the order write API returns for the
  same refusal, without its `ERROR_CODE_` prefix. A canceled order's
  `CancelReason` is a lowercase label such as `user_request`, `ioc_remainder`,
  `stp_expire_maker`, `replaced`, `gtd_expired` or `account_repair`. Both
  are empty for every other status. A value newer than this SDK decodes as
  `UNKNOWN_ERROR_CODE(N)` or `unknown(N)`.
- `decode.OrderCancelReasonLabel` names an `orders.v1.CancelReason`.
- `OrderbookData.Available`, from `Orderbook.Get` and managed order book
  subscriptions. False means the book is being built or was invalidated: its
  levels are unknown, not empty, so request the snapshot again later. An
  environment that does not serve this release yet reports false for every
  book.
- `Trigger.Version`: 1 for a trigger's first state, higher for every later
  state change. Keep the copy with the highest version.
- New order error codes surface as `errors.APIError` with that code. Each
  has a known outcome: `errors.IsRetryable` and
  `errors.MutationOutcomeUnknown` are false, so do not resend the same
  request unchanged.
  - `ERROR_CODE_ATTACHED_LEGS_EXCEEDED`: the order already carries every
    attached leg it may hold, so another attached leg was refused.
  - `ERROR_CODE_FEE_ASSET_INVALID`: the trigger's fee asset was not valid.
  - `ERROR_CODE_FEE_CEILING_INVALID`: the trigger's maximum fee rate was not
    valid.
  - `ERROR_CODE_UNSUPPORTED_FIELD_SET`: the trigger set a field that must be
    left unset.
  - `ERROR_CODE_TRIGGER_FIRE_LIMIT_REACHED`: the trigger could not fire
    again.
  - `ERROR_CODE_CONDITION_FLAGS_INVALID`: the trigger's condition flags were
    not valid.
  - `ERROR_CODE_CONDITION_PARAMETER_SET`: the trigger set a condition
    parameter its condition does not take.
  - `ERROR_CODE_TWAP_CAPACITY_REACHED`: the market had no room for another
    TWAP order. It is a capacity refusal, not a rate limit.
  - `ERROR_CODE_CHILD_ORDER_INCONSISTENT`: the trigger's child order did not
    match the order the trigger requested, so the trigger failed and its
    child order was canceled. Read the trigger and the child order to see
    their final state.

## 0.3.0

Git tag: `v0.3.0`. Released 2026-10-31.

**What a trading bot gains.** Trade, fill, transfer and trigger-event streams
now deliver every event; before, all but one event of each batch was lost.
`MarketData.GetSpotConfig` returns a typed config with each pair's order
limits, including the new cap on one market order's notional, so a bot can
size orders before it sends them. Rejections say more: a duplicate client
order ID names the order that already holds it, a clock-skew rejection is the
retryable `errors.TimestampSkewError`, and an unsupported call is
`errors.NotImplementedError`. Replacement orders report the quantity they
inherited, and slippage caps match what the API accepts.

This release works on devnet today, and on testnet after testnet's next
upgrade, which brings the new private order stream.

**Breaking changes**
- The batched trade, fill, transfer and trigger-event streams emit each event,
  and the single-event decoders (`decode.MarketTradeFromBytes`,
  `UserTradeFromBytes`, `LedgerTransferFromBytes`, `TriggerEventFromBytes`)
  are replaced by their `...BatchFromBytes` versions.
- `decode.SpotConfigFromProto` and `GetSpotConfig` return an error for a spot
  config they cannot decode instead of an empty config.
- Connect `unimplemented` maps to `errors.NotImplementedError` instead of
  `errors.APIError`.

### Added
- `SpotPair.MaxMarketNotionalQuote` and `PairConstraints.MaxMarketNotionalQuote`:
  the largest notional of one market order in quote units, valued at a buy's
  protection price or a sell's reference price. Larger orders are rejected
  with `ERROR_CODE_MAX_NOTIONAL`. Empty when the pair has no cap; a malformed
  value fails the spot config decode and makes the constraints unavailable.
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
