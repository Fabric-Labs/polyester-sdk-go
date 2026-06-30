package codecs

import (
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SubaccountPolicyWriteOpts carries subaccount policy create/update fields.
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
	GlobalPerpLeverageX            *uint32
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

// ApiPolicyWriteOpts carries API policy create/update fields.
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
	req := &authv1.CreateSubaccountPolicyRequest{
		Name:                         opts.Name,
		Description:                  opts.Description,
		SpotMarkets:                  SpotMarketsToProto(opts.SpotMarkets),
		PerpMarkets:                  PerpMarketsToProto(opts.PerpMarkets),
		SpotMarketScope:              spotScope,
		PerpMarketScope:              perpScope,
		Actions:                      actions,
		InternalTransfersOwnOnly:     opts.InternalTransfersOwnOnly,
		EnforceWithdrawWhitelist:     opts.EnforceWithdrawWhitelist,
		TradingHalted:                opts.TradingHalted,
		LiquidationOnly:              opts.LiquidationOnly,
		Locked:                       opts.Locked,
	}
	if subaccountID != nil {
		req.SubaccountId = subaccountID
	}
	applySubaccountPolicyLimits(req, opts)
	if opts.ReviewAt != nil {
		req.ReviewAt = timestamppb.New(opts.ReviewAt.UTC())
	}
	if opts.ExpiresAt != nil {
		req.ExpiresAt = timestamppb.New(opts.ExpiresAt.UTC())
	}
	return req, nil
}

// BuildUpdateSubaccountPolicyRequest builds an update subaccount policy request.
func BuildUpdateSubaccountPolicyRequest(policyID uint64, opts SubaccountPolicyWriteOpts) (*authv1.UpdateSubaccountPolicyRequest, error) {
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
	req := &authv1.UpdateSubaccountPolicyRequest{
		PolicyId:                     policyID,
		Name:                         opts.Name,
		Description:                  opts.Description,
		SpotMarkets:                  SpotMarketsToProto(opts.SpotMarkets),
		PerpMarkets:                  PerpMarketsToProto(opts.PerpMarkets),
		SpotMarketScope:              spotScope,
		PerpMarketScope:              perpScope,
		Actions:                      actions,
		InternalTransfersOwnOnly:     opts.InternalTransfersOwnOnly,
		EnforceWithdrawWhitelist:     opts.EnforceWithdrawWhitelist,
		TradingHalted:                opts.TradingHalted,
		LiquidationOnly:              opts.LiquidationOnly,
		Locked:                       opts.Locked,
	}
	applyUpdateSubaccountPolicyLimits(req, opts)
	if opts.ReviewAt != nil {
		req.ReviewAt = timestamppb.New(opts.ReviewAt.UTC())
	}
	if opts.ExpiresAt != nil {
		req.ExpiresAt = timestamppb.New(opts.ExpiresAt.UTC())
	}
	return req, nil
}

// BuildCreateApiPolicyRequest builds a create API policy request.
func BuildCreateApiPolicyRequest(opts ApiPolicyWriteOpts) (*authv1.CreateApiPolicyRequest, error) {
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
	req := &authv1.CreateApiPolicyRequest{
		Name:            opts.Name,
		Description:     opts.Description,
		SpotMarkets:     SpotMarketsToProto(opts.SpotMarkets),
		PerpMarkets:     PerpMarketsToProto(opts.PerpMarkets),
		SpotMarketScope: spotScope,
		PerpMarketScope: perpScope,
		Actions:         actions,
		IsTemplate:      opts.IsTemplate,
		AssignToKeyId:   opts.AssignToKeyID,
	}
	if opts.MaxOrderNotional != nil {
		req.MaxOrderNotional = *opts.MaxOrderNotional
	}
	if opts.DailyInternalTransferOutLimit != nil {
		req.DailyInternalTransferOutLimit = *opts.DailyInternalTransferOutLimit
	}
	if opts.DailyWithdrawLimit != nil {
		req.DailyWithdrawLimit = *opts.DailyWithdrawLimit
	}
	return req, nil
}

// BuildUpdateApiPolicyRequest builds an update API policy request.
func BuildUpdateApiPolicyRequest(policyID uint64, opts ApiPolicyWriteOpts) (*authv1.UpdateApiPolicyRequest, error) {
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
	req := &authv1.UpdateApiPolicyRequest{
		PolicyId:        policyID,
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
		req.MaxOrderNotional = *opts.MaxOrderNotional
	}
	if opts.DailyInternalTransferOutLimit != nil {
		req.DailyInternalTransferOutLimit = *opts.DailyInternalTransferOutLimit
	}
	if opts.DailyWithdrawLimit != nil {
		req.DailyWithdrawLimit = *opts.DailyWithdrawLimit
	}
	return req, nil
}

func applySubaccountPolicyLimits(req *authv1.CreateSubaccountPolicyRequest, opts SubaccountPolicyWriteOpts) {
	if opts.GlobalNotionalCap != nil {
		req.GlobalNotionalCap = *opts.GlobalNotionalCap
	}
	if opts.MaxOrderNotional != nil {
		req.MaxOrderNotional = *opts.MaxOrderNotional
	}
	if opts.MaxOpenOrders != nil {
		req.MaxOpenOrders = *opts.MaxOpenOrders
	}
	if opts.MaxOpenPositions != nil {
		req.MaxOpenPositions = *opts.MaxOpenPositions
	}
	if opts.GlobalPerpLeverageX != nil {
		req.GlobalPerpLeverageX = *opts.GlobalPerpLeverageX
	}
	if opts.DailyInternalTransferOutLimit != nil {
		req.DailyInternalTransferOutLimit = *opts.DailyInternalTransferOutLimit
	}
	if opts.DailyWithdrawLimit != nil {
		req.DailyWithdrawLimit = *opts.DailyWithdrawLimit
	}
	if opts.DailyLossLimit != nil {
		req.DailyLossLimit = *opts.DailyLossLimit
	}
	if opts.IntradayDrawdownLimitBps != nil {
		req.IntradayDrawdownLimitBps = *opts.IntradayDrawdownLimitBps
	}
}

func applyUpdateSubaccountPolicyLimits(req *authv1.UpdateSubaccountPolicyRequest, opts SubaccountPolicyWriteOpts) {
	if opts.GlobalNotionalCap != nil {
		req.GlobalNotionalCap = *opts.GlobalNotionalCap
	}
	if opts.MaxOrderNotional != nil {
		req.MaxOrderNotional = *opts.MaxOrderNotional
	}
	if opts.MaxOpenOrders != nil {
		req.MaxOpenOrders = *opts.MaxOpenOrders
	}
	if opts.MaxOpenPositions != nil {
		req.MaxOpenPositions = *opts.MaxOpenPositions
	}
	if opts.GlobalPerpLeverageX != nil {
		req.GlobalPerpLeverageX = *opts.GlobalPerpLeverageX
	}
	if opts.DailyInternalTransferOutLimit != nil {
		req.DailyInternalTransferOutLimit = *opts.DailyInternalTransferOutLimit
	}
	if opts.DailyWithdrawLimit != nil {
		req.DailyWithdrawLimit = *opts.DailyWithdrawLimit
	}
	if opts.DailyLossLimit != nil {
		req.DailyLossLimit = *opts.DailyLossLimit
	}
	if opts.IntradayDrawdownLimitBps != nil {
		req.IntradayDrawdownLimitBps = *opts.IntradayDrawdownLimitBps
	}
}
