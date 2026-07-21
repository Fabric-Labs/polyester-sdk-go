package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func subPolicy(msg *authv1.SubaccountPolicyView) *models.SubaccountPolicy {
	if msg == nil {
		return nil
	}
	return &models.SubaccountPolicy{
		PolicyID:    codecs.FormatUint64ID(msg.GetId()),
		Name:        msg.GetName(),
		Description: msg.GetDescription(),
		Revision:    msg.GetRevision(),
	}
}

// SubaccountPolicyMessageFromProto decodes one subaccount policy message.
func SubaccountPolicyMessageFromProto(msg *authv1.SubaccountPolicyView) *models.SubaccountPolicy {
	return subPolicy(msg)
}

func SubaccountPoliciesListFromProto(msg *authv1.ListSubaccountPoliciesResponse) models.SubaccountPoliciesList {
	out := make([]models.SubaccountPolicy, 0, len(msg.GetPolicies()))
	for _, p := range msg.GetPolicies() {
		if row := subPolicy(p); row != nil {
			out = append(out, *row)
		}
	}
	return models.SubaccountPoliciesList{Policies: out}
}

func GetSubaccountPolicyFromProto(msg *authv1.GetSubaccountPolicyResponse) *models.SubaccountPolicy {
	return subPolicy(msg.GetPolicy())
}
func CreateSubaccountPolicyFromProto(msg *authv1.CreateSubaccountPolicyResponse) *models.SubaccountPolicy {
	return subPolicy(msg.GetPolicy())
}
func UpdateSubaccountPolicyFromProto(msg *authv1.UpdateSubaccountPolicyResponse) *models.SubaccountPolicy {
	return subPolicy(msg.GetPolicy())
}

func apiPolicy(msg *authv1.ApiPolicyView) *models.ApiPolicy {
	if msg == nil {
		return nil
	}
	return &models.ApiPolicy{
		PolicyID:    codecs.FormatUint64ID(msg.GetId()),
		Name:        msg.GetName(),
		Description: msg.GetDescription(),
		Revision:    msg.GetRevision(),
	}
}

func ApiPoliciesListFromProto(msg *authv1.ListApiPoliciesResponse) models.ApiPoliciesList {
	out := make([]models.ApiPolicy, 0, len(msg.GetPolicies()))
	for _, p := range msg.GetPolicies() {
		if row := apiPolicy(p); row != nil {
			out = append(out, *row)
		}
	}
	return models.ApiPoliciesList{Policies: out}
}

func GetApiPolicyFromProto(msg *authv1.GetApiPolicyResponse) *models.ApiPolicy {
	return apiPolicy(msg.GetPolicy())
}
func CreateApiPolicyFromProto(msg *authv1.CreateApiPolicyResponse) *models.ApiPolicy {
	return apiPolicy(msg.GetPolicy())
}
func UpdateApiPolicyFromProto(msg *authv1.UpdateApiPolicyResponse) *models.ApiPolicy {
	return apiPolicy(msg.GetPolicy())
}
