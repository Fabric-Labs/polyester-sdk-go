package services

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1/chainlifecyclev1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type LifecycleService struct {
	transport *transport.Factory
	realtime  RealtimeClient
	scoped    ScopedSubAccount
}

// NewLifecycleService builds the lifecycle service. defaultSubAccountID is the
// sub-account ListAccountFlows and GetAccountFlow read when a call selects
// none, as for the other account reads.
func NewLifecycleService(factory *transport.Factory, realtime RealtimeClient, defaultSubAccountID *string) *LifecycleService {
	return &LifecycleService{transport: factory, realtime: realtime, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *LifecycleService) client() chainlifecyclev1connect.LifecycleReadServiceClient {
	return chainlifecyclev1connect.NewLifecycleReadServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *LifecycleService) signedClient() chainlifecyclev1connect.LifecycleReadServiceClient {
	return chainlifecyclev1connect.NewLifecycleReadServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

// ListFlowsParams selects a page of lifecycle flows for ListFlowsWith.
type ListFlowsParams struct {
	// Limit is the page size; 0 uses 50.
	Limit int
	// PageToken continues from a previous page's NextPageToken. A token is
	// bound to the request it was returned for: send it with the same
	// account, participation, scope and order; only Limit may change.
	PageToken *string
	// Scope is "all", "open_only" or "terminal_only"; nil lists all.
	Scope *string
	// SmartAccountAddress selects one account by its Polyester Chain smart
	// account address, its public identity. Without it, the list covers every
	// account. To list the caller's own flows, including those no chain
	// transaction started, use ListAccountFlows.
	SmartAccountAddress *string
	// Participation narrows a list that selects an account: "owner" for the
	// flows it owns, "recipient" for the transfers other accounts sent to it.
	// nil or "" returns both. The server refuses it without an account.
	Participation *string
	// Reversed lists oldest first.
	Reversed bool
}

// ListFlows lists lifecycle flows a chain transaction started, newest first
// unless reversed. It is public: a flow settled without a chain transaction,
// such as a transfer between Trading accounts, is not listed. A list that
// selects a smart account includes the transfers it received as well as the
// flows it owns; each row's Participation says which. Use ListFlowsWith to
// narrow it to one side, and ListAccountFlows for the caller's own flows.
func (s *LifecycleService) ListFlows(ctx context.Context, limit int, pageToken *string, scope *string, smartAccountAddress *string, reversed bool) (models.LifecycleFlowsList, error) {
	return s.ListFlowsWith(ctx, ListFlowsParams{
		Limit:               limit,
		PageToken:           pageToken,
		Scope:               scope,
		SmartAccountAddress: smartAccountAddress,
		Reversed:            reversed,
	})
}

// ListFlowsWith lists lifecycle flows selected by params.
func (s *LifecycleService) ListFlowsWith(ctx context.Context, params ListFlowsParams) (models.LifecycleFlowsList, error) {
	parsedLimit, err := PaginationLimitOrDefault(params.Limit, 50, "limit")
	if err != nil {
		return models.LifecycleFlowsList{}, err
	}
	req := &lifecyclev1.ListFlowsRequest{Limit: parsedLimit}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.LifecycleFlowsList{}, err
	}
	if req.Participation, err = flowParticipationParam(params.Participation); err != nil {
		return models.LifecycleFlowsList{}, err
	}
	req.Sort = flowSort(params.Reversed)
	if params.PageToken != nil {
		req.PageToken = *params.PageToken
	}
	if req.Scope, err = flowListScope(params.Scope); err != nil {
		return models.LifecycleFlowsList{}, err
	}
	if params.SmartAccountAddress != nil {
		req.AccountSelector = &lifecyclev1.ListFlowsRequest_SmartAccountAddress{SmartAccountAddress: *params.SmartAccountAddress}
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().ListFlows, req, decode.FlowsListFromProto)
}

// ListAccountFlowsParams selects a page of the caller's own flows for
// ListAccountFlows.
type ListAccountFlowsParams struct {
	// Account and SubAccountID select the account as for Transfers.List: pass
	// one or neither. With neither, the client's DefaultSubAccountID is used,
	// and without one the account the credentials authenticate (a
	// sub-account-scoped key's own sub-account).
	Account      AccountScope
	SubAccountID *string
	// Limit is the page size; 0 uses 50. At most
	// codecs.ListAccountFlowsLimitMax.
	Limit int
	// PageToken continues from a previous page's NextPageToken. A token is
	// bound to the account and the request it was returned for: send it with
	// the same account, participation, scope and order; only Limit may change.
	PageToken *string
	// Scope is "all", "open_only" or "terminal_only"; nil lists all.
	Scope *string
	// Participation is "owner" for the flows the account owns, "recipient"
	// for the transfers other accounts sent to it; nil or "" returns both.
	Participation *string
	// FlowKind keeps one kind: "deposit", "withdraw" or "transfer"; nil or ""
	// keeps every kind.
	FlowKind *string
	// FlowState keeps one state: "pending_source",
	// "pending_polyester_chain", "pending_ledger", "completed", "failed" or
	// "dropped"; nil or "" keeps every state the scope allows.
	FlowState *string
	// TxRef keeps the flows whose source or latest activity transaction is
	// this transaction identifier, on any supported chain; nil or "" keeps
	// every flow.
	TxRef *string
	// PolyesterChainIDs, ZippedAssetIDs and UnifiedAssetIDs keep the flows of
	// these Polyester Chain IDs, chain asset mapping IDs and unified asset IDs:
	// each at most codecs.ListAccountFlowsFilterIDsMax unique positive IDs;
	// empty keeps every one.
	PolyesterChainIDs []uint32
	ZippedAssetIDs    []uint32
	UnifiedAssetIDs   []uint32
	// OrderBy is the timestamp that orders and pages the list:
	// "last_activity" (the default) or "started_at".
	OrderBy *string
	// Reversed lists oldest first.
	Reversed bool
}

// ListAccountFlows lists the selected account's own lifecycle flows, newest
// first unless reversed: the flows it owns and the transfers it receives,
// including those no chain transaction started, such as a transfer between
// Trading accounts or a Trading withdrawal not yet sent to a chain. Each row's
// Participation says whether the account owns or receives it. The call is
// signed; an API key needs READ_FUNDING_FLOWS on its policy.
func (s *LifecycleService) ListAccountFlows(ctx context.Context, params ListAccountFlowsParams) (models.LifecycleFlowsList, error) {
	req, err := s.accountFlowsRequest(params)
	if err != nil {
		return models.LifecycleFlowsList{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.signedClient().ListAccountFlows, req, decode.AccountFlowsListFromProto)
}

// AccountFlows iterates the selected account's own flows from
// params.PageToken (or the first page), following each page's NextPageToken
// with the same request. It stops at the first error.
func (s *LifecycleService) AccountFlows(ctx context.Context, params ListAccountFlowsParams) iter.Seq2[models.LifecycleFlowSummary, error] {
	start := ""
	if params.PageToken != nil {
		start = *params.PageToken
	}
	return paginate(start, func(token string) ([]models.LifecycleFlowSummary, string, error) {
		page := params
		page.PageToken = &token
		list, err := s.ListAccountFlows(ctx, page)
		return list.Flows, list.NextPageToken, err
	})
}

func (s *LifecycleService) accountFlowsRequest(params ListAccountFlowsParams) (*lifecyclev1.ListAccountFlowsRequest, error) {
	parsedLimit, err := PaginationLimitOrDefault(params.Limit, 50, "limit")
	if err != nil {
		return nil, err
	}
	req := &lifecyclev1.ListAccountFlowsRequest{Limit: parsedLimit, Sort: flowSort(params.Reversed)}
	if err := codecs.CheckPageLimit(req); err != nil {
		return nil, err
	}
	if req.Participation, err = flowParticipationParam(params.Participation); err != nil {
		return nil, err
	}
	if req.Scope, err = flowListScope(params.Scope); err != nil {
		return nil, err
	}
	if params.PageToken != nil {
		req.PageToken = *params.PageToken
	}
	if req.FlowKind, err = enumParam[lifecyclev1.FlowKind](params.FlowKind, "KIND_", lifecyclev1.FlowKind_value, "flow_kind", "'deposit', 'withdraw' or 'transfer'"); err != nil {
		return nil, err
	}
	if req.FlowState, err = enumParam[lifecyclev1.FlowState](params.FlowState, "STATE_", lifecyclev1.FlowState_value, "flow_state",
		"'pending_source', 'pending_polyester_chain', 'pending_ledger', 'completed', 'failed' or 'dropped'"); err != nil {
		return nil, err
	}
	if req.OrderBy, err = enumParam[lifecyclev1.ListOrderBy](params.OrderBy, "ORDER_BY_", lifecyclev1.ListOrderBy_value, "order_by", "'last_activity' or 'started_at'"); err != nil {
		return nil, err
	}
	if params.TxRef != nil {
		req.TxRef = *params.TxRef
	}
	for _, ids := range []struct {
		name string
		in   []uint32
		out  *[]uint32
	}{
		{"polyester_chain_ids", params.PolyesterChainIDs, &req.PolyesterChainIds},
		{"zipped_asset_ids", params.ZippedAssetIDs, &req.ZippedAssetIds},
		{"unified_asset_ids", params.UnifiedAssetIDs, &req.UnifiedAssetIds},
	} {
		if err := checkFilterIDs(ids.name, ids.in); err != nil {
			return nil, err
		}
		*ids.out = append([]uint32(nil), ids.in...)
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, params.Account, params.SubAccountID); err != nil {
		return nil, err
	}
	return req, nil
}

// enumParam reads a lowercase label such as "deposit", in any case and with
// or without its prefix; nil or "" is the zero value.
func enumParam[E ~int32](label *string, prefix string, values map[string]int32, field, allowed string) (E, error) {
	if label == nil || strings.TrimSpace(*label) == "" {
		return 0, nil
	}
	name := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(*label)), prefix)
	if v, ok := values[prefix+name]; ok && v != 0 {
		return E(v), nil
	}
	return 0, &errors.ValidationError{Msg: field + " must be one of " + allowed}
}

// checkFilterIDs refuses an ID filter the contract refuses: more than
// codecs.ListAccountFlowsFilterIDsMax IDs, a zero ID or a repeated one.
func checkFilterIDs(name string, ids []uint32) error {
	if len(ids) > codecs.ListAccountFlowsFilterIDsMax {
		return &errors.ValidationError{Msg: fmt.Sprintf("%s holds at most %d IDs, not %d", name, codecs.ListAccountFlowsFilterIDsMax, len(ids))}
	}
	seen := make(map[uint32]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			return &errors.ValidationError{Msg: name + " IDs must be positive"}
		}
		if _, dup := seen[id]; dup {
			return &errors.ValidationError{Msg: fmt.Sprintf("%s repeats %d", name, id)}
		}
		seen[id] = struct{}{}
	}
	return nil
}

// GetAccountFlow reads one flow the selected account owns or receives,
// including one no chain transaction started; any other flow is not found.
// Account and subAccountID select the account as for ListAccountFlows. The
// call is signed; an API key needs READ_FUNDING_FLOWS on its policy.
func (s *LifecycleService) GetAccountFlow(ctx context.Context, flowID string, account AccountScope, subAccountID *string) (models.LifecycleFlowSummary, error) {
	if flowID == "" {
		return models.LifecycleFlowSummary{}, &errors.ValidationError{Msg: "flow_id is required"}
	}
	req := &lifecyclev1.GetAccountFlowRequest{FlowId: flowID}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.LifecycleFlowSummary{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.signedClient().GetAccountFlow, req, decode.AccountFlowFromGetResponse)
}

func flowSort(reversed bool) lifecyclev1.Sort {
	if reversed {
		return lifecyclev1.Sort_SORT_OLDEST
	}
	return lifecyclev1.Sort_SORT_NEWEST
}

func flowListScope(scope *string) (lifecyclev1.ListScope, error) {
	if scope == nil {
		return lifecyclev1.ListScope_LIST_UNSPECIFIED, nil
	}
	name := strings.ToUpper(*scope)
	if !strings.HasPrefix(name, "LIST_") {
		name = "LIST_" + name
	}
	if v, ok := lifecyclev1.ListScope_value[name]; ok {
		return lifecyclev1.ListScope(v), nil
	}
	return 0, &errors.ValidationError{Msg: "scope must be one of 'all', 'open_only', or 'terminal_only'"}
}

func flowParticipationParam(label *string) (lifecyclev1.FlowParticipation, error) {
	if label == nil {
		return lifecyclev1.FlowParticipation_PARTICIPATION_UNSPECIFIED, nil
	}
	return flowParticipationFromLabel(*label)
}

// flowParticipationFromLabel reads "owner" or "recipient", in any case and
// with or without the PARTICIPATION_ prefix; "" is unspecified.
func flowParticipationFromLabel(label string) (lifecyclev1.FlowParticipation, error) {
	name := strings.ToUpper(strings.TrimSpace(label))
	if name == "" {
		return lifecyclev1.FlowParticipation_PARTICIPATION_UNSPECIFIED, nil
	}
	name = strings.TrimPrefix(name, "PARTICIPATION_")
	switch name {
	case "OWNER":
		return lifecyclev1.FlowParticipation_PARTICIPATION_OWNER, nil
	case "RECIPIENT":
		return lifecyclev1.FlowParticipation_PARTICIPATION_RECIPIENT, nil
	}
	return 0, &errors.ValidationError{Msg: "participation must be one of 'owner' or 'recipient'"}
}

func (s *LifecycleService) GetFlow(ctx context.Context, intentID, flowID *string) (models.LifecycleFlowSummary, error) {
	resolved := flowID
	if resolved == nil || *resolved == "" {
		resolved = intentID
	}
	if resolved == nil || *resolved == "" {
		return models.LifecycleFlowSummary{}, &errors.ValidationError{Msg: "flow_id or intent_id is required"}
	}
	req := &lifecyclev1.GetFlowByIdRequest{FlowId: *resolved}
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetFlowById, req, decode.FlowFromGetResponse)
}

// GetFlowByTx returns all matches in the first response page for a transaction.
// A chain transaction can contain multiple bundled lifecycle flows. Use
// ListFlowsByTx to follow NextPageToken when the result spans multiple pages.
func (s *LifecycleService) GetFlowByTx(ctx context.Context, txHash, lookupKind string, limit int) (models.LifecycleFlowsList, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.LifecycleFlowsList{}, err
	}
	req := &lifecyclev1.ListFlowsByTxRequest{
		TxHash:     txHash,
		LookupKind: codecs.TxLookupKindFromLabel(lookupKind),
		Limit:      parsedLimit,
	}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.LifecycleFlowsList{}, err
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().ListFlowsByTx, req, decode.FlowFromGetByTxResponse)
}

func (s *LifecycleService) ListFlowsByTx(ctx context.Context, txHash, lookupKind string, limit int, pageToken *string) (models.LifecycleFlowsList, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.LifecycleFlowsList{}, err
	}
	req := &lifecyclev1.ListFlowsByTxRequest{
		TxHash:     txHash,
		LookupKind: codecs.TxLookupKindFromLabel(lookupKind),
		Limit:      parsedLimit,
	}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.LifecycleFlowsList{}, err
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().ListFlowsByTx, req, decode.FlowsByTxListFromProto)
}

func (s *LifecycleService) SubscribeOpenFlows(ctx context.Context, accountID any) (*realtime.Subscription[models.LifecycleFlowSummary], error) {
	id, _, err := publicID(accountID, "account_id")
	if err != nil {
		return nil, err
	}
	if id != "" {
		return SubscribeAccountProto(ctx, s.realtime, "private:chain:lifecycle:flows:{account_id}:proto", id, nil, decode.FlowSummaryFromBytes)
	}
	return SubscribePublicProto(ctx, s.realtime, "public:chain:lifecycle:flows:proto", decode.FlowSummaryFromBytes)
}

func (s *LifecycleService) SubscribeFlowDetail(ctx context.Context, flowID string) (*realtime.Subscription[models.LifecycleFlowSummary], error) {
	channel := fmt.Sprintf("public:chain:lifecycle:flow:%s:proto", flowID)
	return SubscribePublicProto(ctx, s.realtime, channel, decode.FlowDetailFromBytes)
}
