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

// ApiPolicyMessageFromProto decodes one API key policy message.
func ApiPolicyMessageFromProto(msg *authv1.ApiPolicyView) *models.ApiPolicy {
	return apiPolicy(msg)
}
