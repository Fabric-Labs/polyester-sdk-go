package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func spotMarketRules(rules []*authv1.SpotMarketRule, cats *catalogs.Manager) []models.SpotMarketRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]models.SpotMarketRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		id := rule.GetSymbolId()
		out = append(out, models.SpotMarketRule{
			SymbolID: id,
			Symbol:   catalogSymbol(cats, id),
		})
	}
	return out
}

func policyActions(actions []authv1.PolicyAction) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		if action == authv1.PolicyAction_UNSPECIFIED {
			continue
		}
		out = append(out, action.String())
	}
	return out
}

func marketScopeLabel(scope authv1.MarketScope_Value) string {
	switch scope {
	case authv1.MarketScope_ALL:
		return "ALL"
	case authv1.MarketScope_ALLOWLIST:
		return "ALLOWLIST"
	default:
		return ""
	}
}

func sourceTemplateID(id uint64) string {
	if id == 0 {
		return ""
	}
	return codecs.FormatUint64ID(id)
}

// SubaccountPolicyFromView decodes one subaccount policy view.
func SubaccountPolicyFromView(msg *authv1.SubaccountPolicyView, cats *catalogs.Manager) *models.SubaccountPolicy {
	if msg == nil {
		return nil
	}
	return &models.SubaccountPolicy{
		PolicyID:         codecs.FormatUint64ID(msg.GetId()),
		Name:             msg.GetName(),
		Description:      msg.GetDescription(),
		Revision:         msg.GetRevision(),
		SpotMarkets:      spotMarketRules(msg.GetSpotMarkets(), cats),
		SpotMarketScope:  marketScopeLabel(msg.GetSpotMarketScope()),
		Actions:          policyActions(msg.GetActions()),
		IsTemplate:       msg.GetIsTemplate(),
		SourceTemplateID: sourceTemplateID(msg.GetSourceTemplateId()),
		MaxOrderNotional: msg.GetMaxOrderNotional(),
		MaxOpenOrders:    msg.GetMaxOpenOrders(),
		TradingHalted:    msg.GetTradingHalted(),
		Locked:           msg.GetLocked(),
		ReviewAt:         timestampTime(msg.GetReviewAt()),
		ExpiresAt:        timestampTime(msg.GetExpiresAt()),
		CreatedAt:        timestampTime(msg.GetCreatedAt()),
		UpdatedAt:        timestampTime(msg.GetUpdatedAt()),
	}
}

// SubaccountPolicyMessageFromProto decodes one subaccount policy message.
func SubaccountPolicyMessageFromProto(msg *authv1.SubaccountPolicyView) *models.SubaccountPolicy {
	return SubaccountPolicyFromView(msg, nil)
}

// ApiPolicyFromView decodes one API key policy view.
func ApiPolicyFromView(msg *authv1.ApiPolicyView, cats *catalogs.Manager) *models.ApiPolicy {
	if msg == nil {
		return nil
	}
	return &models.ApiPolicy{
		PolicyID:         codecs.FormatUint64ID(msg.GetId()),
		Name:             msg.GetName(),
		Description:      msg.GetDescription(),
		Revision:         msg.GetRevision(),
		SpotMarkets:      spotMarketRules(msg.GetSpotMarkets(), cats),
		SpotMarketScope:  marketScopeLabel(msg.GetSpotMarketScope()),
		Actions:          policyActions(msg.GetActions()),
		IsTemplate:       msg.GetIsTemplate(),
		SourceTemplateID: sourceTemplateID(msg.GetSourceTemplateId()),
		CreatedAt:        timestampTime(msg.GetCreatedAt()),
		UpdatedAt:        timestampTime(msg.GetUpdatedAt()),
	}
}

// ApiPolicyMessageFromProto decodes one API key policy message.
func ApiPolicyMessageFromProto(msg *authv1.ApiPolicyView) *models.ApiPolicy {
	return ApiPolicyFromView(msg, nil)
}
