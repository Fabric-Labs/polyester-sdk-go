package decode

import (
	"fmt"

	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// RateLimitDetailFromProto decodes polyester.ratelimit.v1.RateLimitDetail.
func RateLimitDetailFromProto(msg *ratelimitv1.RateLimitDetail) *models.RateLimitDetail {
	if msg == nil {
		return nil
	}
	out := &models.RateLimitDetail{
		Reason:      enumNameOrUnknown(ratelimitv1.FailureReason_name, int32(msg.GetReason()), "UNKNOWN_FAILURE_REASON"),
		OperationID: msg.GetOperationId(),
		PolicyClass: enumNameOrUnknown(ratelimitv1.PolicyClass_name, int32(msg.GetPolicyClass()), "UNKNOWN_POLICY_CLASS"),
		Scope:       enumNameOrUnknown(ratelimitv1.LimiterScope_name, int32(msg.GetScope()), "UNKNOWN_LIMITER_SCOPE"),
		RefillModel: enumNameOrUnknown(ratelimitv1.RefillModel_name, int32(msg.GetRefillModel()), "UNKNOWN_REFILL_MODEL"),
	}
	if msg.Limit != nil {
		v := msg.GetLimit()
		out.Limit = &v
	}
	if msg.Remaining != nil {
		v := msg.GetRemaining()
		out.Remaining = &v
	}
	if msg.RetryAfterMs != nil {
		v := msg.GetRetryAfterMs()
		out.RetryAfterMs = &v
	}
	if msg.PolicyVersion != nil {
		v := msg.GetPolicyVersion()
		out.PolicyVersion = &v
	}
	return out
}

func enumNameOrUnknown(names map[int32]string, raw int32, unknownPrefix string) string {
	if name, ok := names[raw]; ok {
		return name
	}
	return fmt.Sprintf("%s(%d)", unknownPrefix, raw)
}
