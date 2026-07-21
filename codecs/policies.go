package codecs

import (
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SubaccountPolicyWriteOpts carries full create fields for a subaccount policy.
type SubaccountPolicyWriteOpts struct {
	Name                          string
	Description                   string
	SpotMarkets                   []models.SpotMarketRule
	PerpMarkets                   []models.PerpMarketRule
	SpotMarketScope               string
	PerpMarketScope               string
	Actions                       []string
	GlobalNotionalCap             *uint64
	MaxOrderNotional              *uint64
	MaxOpenOrders                 *uint32
	MaxOpenPositions              *uint32
	GlobalPerpLeverageX           *uint32
	DailyInternalTransferOutLimit *uint64
	DailyWithdrawLimit            *uint64
	InternalTransfersOwnOnly      bool
	EnforceWithdrawWhitelist      bool
	TradingHalted                 bool
	LiquidationOnly               bool
	DailyLossLimit                *uint64
	IntradayDrawdownLimitBps      *uint32
	Locked                        bool
	ReviewAt                      *time.Time
	ExpiresAt                     *time.Time
}

// SubaccountPolicyPatch carries optional update fields. Nil pointer = omit;
// non-nil (including zero/empty/false) = set and include in the field mask.
type SubaccountPolicyPatch struct {
	ExpectedRevision              uint64
	Name                          *string
	Description                   *string
	SpotMarkets                   *[]models.SpotMarketRule
	PerpMarkets                   *[]models.PerpMarketRule
	SpotMarketScope               *string
	PerpMarketScope               *string
	Actions                       *[]string
	GlobalNotionalCap             *uint64
	MaxOrderNotional              *uint64
	MaxOpenOrders                 *uint32
	MaxOpenPositions              *uint32
	GlobalPerpLeverageX           *uint32
	DailyInternalTransferOutLimit *uint64
	DailyWithdrawLimit            *uint64
	InternalTransfersOwnOnly      *bool
	EnforceWithdrawWhitelist      *bool
	TradingHalted                 *bool
	LiquidationOnly               *bool
	DailyLossLimit                *uint64
	IntradayDrawdownLimitBps      *uint32
	Locked                        *bool
	ReviewAt                      *ClearableTime
	ExpiresAt                     *ClearableTime
}

// ApiPolicyWriteOpts carries full create fields for an API policy.
type ApiPolicyWriteOpts struct {
	Name                          string
	Description                   string
	SpotMarkets                   []models.SpotMarketRule
	PerpMarkets                   []models.PerpMarketRule
	SpotMarketScope               string
	PerpMarketScope               string
	Actions                       []string
	MaxOrderNotional              *uint64
	DailyInternalTransferOutLimit *uint64
	DailyWithdrawLimit            *uint64
	IsTemplate                    bool
	AssignToKeyID                 string
}

// ApiPolicyPatch carries optional update fields for an API policy.
type ApiPolicyPatch struct {
	ExpectedRevision              uint64
	Name                          *string
	Description                   *string
	SpotMarkets                   *[]models.SpotMarketRule
	PerpMarkets                   *[]models.PerpMarketRule
	SpotMarketScope               *string
	PerpMarketScope               *string
	Actions                       *[]string
	MaxOrderNotional              *uint64
	DailyInternalTransferOutLimit *uint64
	DailyWithdrawLimit            *uint64
	IsTemplate                    *bool
}

// MarketScopeFromLabel maps a market scope label to the proto enum.
func MarketScopeFromLabel(value string) (authv1.MarketScope_Value, error) {
	if value == "" {
		return authv1.MarketScope_ALL, nil
	}
	normalized := strings.ToLower(strings.ReplaceAll(value, "-", "_"))
	switch normalized {
	case "all", "unspecified":
		return authv1.MarketScope_ALL, nil
	case "allowlist":
		return authv1.MarketScope_ALLOWLIST, nil
	default:
		return 0, &errors.ValidationError{Msg: "market scope must be 'all' or 'allowlist'"}
	}
}

// PolicyActionFromLabel maps a policy action label to the proto enum.
func PolicyActionFromLabel(value string) (authv1.PolicyAction, error) {
	normalized := strings.ToLower(strings.ReplaceAll(value, "-", "_"))
	normalized = strings.TrimPrefix(normalized, "policy_action_")
	if v, ok := authv1.PolicyAction_value[strings.ToUpper(normalized)]; ok && v != 0 {
		return authv1.PolicyAction(v), nil
	}
	return 0, &errors.ValidationError{Msg: "unknown policy action: " + value}
}

// PolicyActionsFromLabels maps policy action labels to proto enums.
func PolicyActionsFromLabels(values []string) ([]authv1.PolicyAction, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]authv1.PolicyAction, 0, len(values))
	for _, value := range values {
		action, err := PolicyActionFromLabel(value)
		if err != nil {
			return nil, err
		}
		out = append(out, action)
	}
	return out, nil
}

// SpotMarketsToProto converts spot market rules to proto.
func SpotMarketsToProto(values []models.SpotMarketRule) []*authv1.SpotMarketRule {
	if len(values) == 0 {
		return nil
	}
	out := make([]*authv1.SpotMarketRule, 0, len(values))
	for _, item := range values {
		out = append(out, &authv1.SpotMarketRule{Symbol: item.Symbol})
	}
	return out
}

// PerpMarketsToProto converts perp market rules to proto.
func PerpMarketsToProto(values []models.PerpMarketRule) []*authv1.PerpMarketRule {
	if len(values) == 0 {
		return nil
	}
	out := make([]*authv1.PerpMarketRule, 0, len(values))
	for _, item := range values {
		rule := &authv1.PerpMarketRule{Symbol: item.Symbol}
		if item.MaxLeverageX != 0 {
			rule.MaxLeverageX = uint32(item.MaxLeverageX)
		}
		out = append(out, rule)
	}
	return out
}

// BuildCreateSubaccountPolicyRequest builds a create subaccount policy request.
func BuildCreateSubaccountPolicyRequest(subaccountID *uint64, opts SubaccountPolicyWriteOpts) (*authv1.CreateSubaccountPolicyRequest, error) {
	spec, err := buildSubaccountPolicySpec(opts)
	if err != nil {
		return nil, err
	}
	req := &authv1.CreateSubaccountPolicyRequest{Policy: spec}
	if subaccountID != nil {
		req.SubaccountId = subaccountID
	}
	return req, nil
}

// BuildUpdateSubaccountPolicyRequest builds an update subaccount policy request.
func BuildUpdateSubaccountPolicyRequest(policyID uint64, patch SubaccountPolicyPatch) (*authv1.UpdateSubaccountPolicyRequest, error) {
	if err := requirePositiveRevision(patch.ExpectedRevision); err != nil {
		return nil, err
	}
	spec := &authv1.SubaccountPolicySpec{}
	var paths []string

	if patch.Name != nil {
		spec.Name = *patch.Name
		paths = append(paths, "name")
	}
	if patch.Description != nil {
		spec.Description = *patch.Description
		paths = append(paths, "description")
	}
	if patch.SpotMarkets != nil {
		spec.SpotMarkets = SpotMarketsToProto(*patch.SpotMarkets)
		paths = append(paths, "spot_markets")
	}
	if patch.PerpMarkets != nil {
		spec.PerpMarkets = PerpMarketsToProto(*patch.PerpMarkets)
		paths = append(paths, "perp_markets")
	}
	if patch.SpotMarketScope != nil {
		scope, err := MarketScopeFromLabel(*patch.SpotMarketScope)
		if err != nil {
			return nil, err
		}
		spec.SpotMarketScope = scope
		paths = append(paths, "spot_market_scope")
	}
	if patch.PerpMarketScope != nil {
		scope, err := MarketScopeFromLabel(*patch.PerpMarketScope)
		if err != nil {
			return nil, err
		}
		spec.PerpMarketScope = scope
		paths = append(paths, "perp_market_scope")
	}
	if patch.Actions != nil {
		actions, err := PolicyActionsFromLabels(*patch.Actions)
		if err != nil {
			return nil, err
		}
		spec.Actions = actions
		paths = append(paths, "actions")
	}
	if patch.GlobalNotionalCap != nil {
		spec.GlobalNotionalCap = *patch.GlobalNotionalCap
		paths = append(paths, "global_notional_cap")
	}
	if patch.MaxOrderNotional != nil {
		spec.MaxOrderNotional = *patch.MaxOrderNotional
		paths = append(paths, "max_order_notional")
	}
	if patch.MaxOpenOrders != nil {
		spec.MaxOpenOrders = *patch.MaxOpenOrders
		paths = append(paths, "max_open_orders")
	}
	if patch.MaxOpenPositions != nil {
		spec.MaxOpenPositions = *patch.MaxOpenPositions
		paths = append(paths, "max_open_positions")
	}
	if patch.GlobalPerpLeverageX != nil {
		spec.GlobalPerpLeverageX = *patch.GlobalPerpLeverageX
		paths = append(paths, "global_perp_leverage_x")
	}
	if patch.DailyInternalTransferOutLimit != nil {
		spec.DailyInternalTransferOutLimit = *patch.DailyInternalTransferOutLimit
		paths = append(paths, "daily_internal_transfer_out_limit")
	}
	if patch.DailyWithdrawLimit != nil {
		spec.DailyWithdrawLimit = *patch.DailyWithdrawLimit
		paths = append(paths, "daily_withdraw_limit")
	}
	if patch.InternalTransfersOwnOnly != nil {
		spec.InternalTransfersOwnOnly = *patch.InternalTransfersOwnOnly
		paths = append(paths, "internal_transfers_own_only")
	}
	if patch.EnforceWithdrawWhitelist != nil {
		spec.EnforceWithdrawWhitelist = *patch.EnforceWithdrawWhitelist
		paths = append(paths, "enforce_withdraw_whitelist")
	}
	if patch.TradingHalted != nil {
		spec.TradingHalted = *patch.TradingHalted
		paths = append(paths, "trading_halted")
	}
	if patch.LiquidationOnly != nil {
		spec.LiquidationOnly = *patch.LiquidationOnly
		paths = append(paths, "liquidation_only")
	}
	if patch.DailyLossLimit != nil {
		spec.DailyLossLimit = *patch.DailyLossLimit
		paths = append(paths, "daily_loss_limit")
	}
	if patch.IntradayDrawdownLimitBps != nil {
		spec.IntradayDrawdownLimitBps = *patch.IntradayDrawdownLimitBps
		paths = append(paths, "intraday_drawdown_limit_bps")
	}
	if patch.Locked != nil {
		spec.Locked = *patch.Locked
		paths = append(paths, "locked")
	}
	if patch.ReviewAt != nil {
		ts, err := timestampFromClearable(patch.ReviewAt, "review_at")
		if err != nil {
			return nil, err
		}
		spec.ReviewAt = ts
		paths = append(paths, "review_at")
	}
	if patch.ExpiresAt != nil {
		ts, err := timestampFromClearable(patch.ExpiresAt, "expires_at")
		if err != nil {
			return nil, err
		}
		spec.ExpiresAt = ts
		paths = append(paths, "expires_at")
	}

	mask, err := newUpdateMask(paths)
	if err != nil {
		return nil, err
	}
	return &authv1.UpdateSubaccountPolicyRequest{
		PolicyId:         policyID,
		Policy:           spec,
		UpdateMask:       mask,
		ExpectedRevision: patch.ExpectedRevision,
	}, nil
}

// BuildCreateApiPolicyRequest builds a create API policy request.
func BuildCreateApiPolicyRequest(opts ApiPolicyWriteOpts) (*authv1.CreateApiPolicyRequest, error) {
	spec, err := buildApiPolicySpec(opts)
	if err != nil {
		return nil, err
	}
	return &authv1.CreateApiPolicyRequest{
		Policy:        spec,
		AssignToKeyId: opts.AssignToKeyID,
	}, nil
}

// BuildUpdateApiPolicyRequest builds an update API policy request.
func BuildUpdateApiPolicyRequest(policyID uint64, patch ApiPolicyPatch) (*authv1.UpdateApiPolicyRequest, error) {
	if err := requirePositiveRevision(patch.ExpectedRevision); err != nil {
		return nil, err
	}
	spec := &authv1.ApiPolicySpec{}
	var paths []string

	if patch.Name != nil {
		spec.Name = *patch.Name
		paths = append(paths, "name")
	}
	if patch.Description != nil {
		spec.Description = *patch.Description
		paths = append(paths, "description")
	}
	if patch.SpotMarkets != nil {
		spec.SpotMarkets = SpotMarketsToProto(*patch.SpotMarkets)
		paths = append(paths, "spot_markets")
	}
	if patch.PerpMarkets != nil {
		spec.PerpMarkets = PerpMarketsToProto(*patch.PerpMarkets)
		paths = append(paths, "perp_markets")
	}
	if patch.SpotMarketScope != nil {
		scope, err := MarketScopeFromLabel(*patch.SpotMarketScope)
		if err != nil {
			return nil, err
		}
		spec.SpotMarketScope = scope
		paths = append(paths, "spot_market_scope")
	}
	if patch.PerpMarketScope != nil {
		scope, err := MarketScopeFromLabel(*patch.PerpMarketScope)
		if err != nil {
			return nil, err
		}
		spec.PerpMarketScope = scope
		paths = append(paths, "perp_market_scope")
	}
	if patch.Actions != nil {
		actions, err := PolicyActionsFromLabels(*patch.Actions)
		if err != nil {
			return nil, err
		}
		spec.Actions = actions
		paths = append(paths, "actions")
	}
	if patch.MaxOrderNotional != nil {
		spec.MaxOrderNotional = *patch.MaxOrderNotional
		paths = append(paths, "max_order_notional")
	}
	if patch.DailyInternalTransferOutLimit != nil {
		spec.DailyInternalTransferOutLimit = *patch.DailyInternalTransferOutLimit
		paths = append(paths, "daily_internal_transfer_out_limit")
	}
	if patch.DailyWithdrawLimit != nil {
		spec.DailyWithdrawLimit = *patch.DailyWithdrawLimit
		paths = append(paths, "daily_withdraw_limit")
	}
	if patch.IsTemplate != nil {
		spec.IsTemplate = *patch.IsTemplate
		paths = append(paths, "is_template")
	}

	mask, err := newUpdateMask(paths)
	if err != nil {
		return nil, err
	}
	return &authv1.UpdateApiPolicyRequest{
		PolicyId:         policyID,
		Policy:           spec,
		UpdateMask:       mask,
		ExpectedRevision: patch.ExpectedRevision,
	}, nil
}

func buildSubaccountPolicySpec(opts SubaccountPolicyWriteOpts) (*authv1.SubaccountPolicySpec, error) {
	spotScope, err := MarketScopeFromLabel(opts.SpotMarketScope)
	if err != nil {
		return nil, err
	}
	perpScope, err := MarketScopeFromLabel(opts.PerpMarketScope)
	if err != nil {
		return nil, err
	}
	actions, err := PolicyActionsFromLabels(opts.Actions)
	if err != nil {
		return nil, err
	}
	spec := &authv1.SubaccountPolicySpec{
		Name:                     opts.Name,
		Description:              opts.Description,
		SpotMarkets:              SpotMarketsToProto(opts.SpotMarkets),
		PerpMarkets:              PerpMarketsToProto(opts.PerpMarkets),
		SpotMarketScope:          spotScope,
		PerpMarketScope:          perpScope,
		Actions:                  actions,
		InternalTransfersOwnOnly: opts.InternalTransfersOwnOnly,
		EnforceWithdrawWhitelist: opts.EnforceWithdrawWhitelist,
		TradingHalted:            opts.TradingHalted,
		LiquidationOnly:          opts.LiquidationOnly,
		Locked:                   opts.Locked,
	}
	if opts.GlobalNotionalCap != nil {
		spec.GlobalNotionalCap = *opts.GlobalNotionalCap
	}
	if opts.MaxOrderNotional != nil {
		spec.MaxOrderNotional = *opts.MaxOrderNotional
	}
	if opts.MaxOpenOrders != nil {
		spec.MaxOpenOrders = *opts.MaxOpenOrders
	}
	if opts.MaxOpenPositions != nil {
		spec.MaxOpenPositions = *opts.MaxOpenPositions
	}
	if opts.GlobalPerpLeverageX != nil {
		spec.GlobalPerpLeverageX = *opts.GlobalPerpLeverageX
	}
	if opts.DailyInternalTransferOutLimit != nil {
		spec.DailyInternalTransferOutLimit = *opts.DailyInternalTransferOutLimit
	}
	if opts.DailyWithdrawLimit != nil {
		spec.DailyWithdrawLimit = *opts.DailyWithdrawLimit
	}
	if opts.DailyLossLimit != nil {
		spec.DailyLossLimit = *opts.DailyLossLimit
	}
	if opts.IntradayDrawdownLimitBps != nil {
		spec.IntradayDrawdownLimitBps = *opts.IntradayDrawdownLimitBps
	}
	if opts.ReviewAt != nil {
		spec.ReviewAt = timestamppb.New(opts.ReviewAt.UTC())
	}
	if opts.ExpiresAt != nil {
		spec.ExpiresAt = timestamppb.New(opts.ExpiresAt.UTC())
	}
	return spec, nil
}

func buildApiPolicySpec(opts ApiPolicyWriteOpts) (*authv1.ApiPolicySpec, error) {
	spotScope, err := MarketScopeFromLabel(opts.SpotMarketScope)
	if err != nil {
		return nil, err
	}
	perpScope, err := MarketScopeFromLabel(opts.PerpMarketScope)
	if err != nil {
		return nil, err
	}
	actions, err := PolicyActionsFromLabels(opts.Actions)
	if err != nil {
		return nil, err
	}
	spec := &authv1.ApiPolicySpec{
		Name:            opts.Name,
		Description:     opts.Description,
		SpotMarkets:     SpotMarketsToProto(opts.SpotMarkets),
		PerpMarkets:     PerpMarketsToProto(opts.PerpMarkets),
		SpotMarketScope: spotScope,
		PerpMarketScope: perpScope,
		Actions:         actions,
		IsTemplate:      opts.IsTemplate,
	}
	if opts.MaxOrderNotional != nil {
		spec.MaxOrderNotional = *opts.MaxOrderNotional
	}
	if opts.DailyInternalTransferOutLimit != nil {
		spec.DailyInternalTransferOutLimit = *opts.DailyInternalTransferOutLimit
	}
	if opts.DailyWithdrawLimit != nil {
		spec.DailyWithdrawLimit = *opts.DailyWithdrawLimit
	}
	return spec, nil
}
