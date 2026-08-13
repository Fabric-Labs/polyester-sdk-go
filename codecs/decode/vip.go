package decode

import (
	vipv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/vip/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func cloneOptionalString(src *string) *string {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

// VIPTierFromProto decodes one VIP catalog row. AopThresholdUsd stays nil when unset (VIP0).
func VIPTierFromProto(msg *vipv1.VIPTier) models.VIPTier {
	if msg == nil {
		return models.VIPTier{}
	}
	return models.VIPTier{
		Tier:                msg.GetTier(),
		VolumeThresholdUsd:  msg.GetVolumeThresholdUsd(),
		AopThresholdUsd:     cloneOptionalString(msg.AopThresholdUsd),
		MakerFeeRatePercent: msg.GetMakerFeeRatePercent(),
		TakerFeeRatePercent: msg.GetTakerFeeRatePercent(),
	}
}

// VIPTiersListFromProto decodes ListVIPTiersResponse.
func VIPTiersListFromProto(msg *vipv1.ListVIPTiersResponse) models.VIPTiersList {
	tiers := msg.GetTiers()
	out := make([]models.VIPTier, 0, len(tiers))
	for _, item := range tiers {
		if item == nil {
			continue
		}
		out = append(out, VIPTierFromProto(item))
	}
	return models.VIPTiersList{
		PolicyVersion:        msg.GetPolicyVersion(),
		EffectiveFrom:        timestampTime(msg.GetEffectiveFrom()),
		RetentionThresholdBp: msg.GetRetentionThresholdBp(),
		Tiers:                out,
	}
}

// NextVIPTierThresholdsFromProto decodes next-tier entry thresholds.
func NextVIPTierThresholdsFromProto(msg *vipv1.NextVIPTierThresholds) models.NextVIPTierThresholds {
	if msg == nil {
		return models.NextVIPTierThresholds{}
	}
	return models.NextVIPTierThresholds{
		Tier:               msg.GetTier(),
		VolumeThresholdUsd: msg.GetVolumeThresholdUsd(),
		AopThresholdUsd:    msg.GetAopThresholdUsd(),
	}
}

// VIPStatusFromProto decodes GetVIPStatusResponse. Optional oneofs stay nil when unset.
func VIPStatusFromProto(msg *vipv1.GetVIPStatusResponse) models.VIPStatus {
	if msg == nil {
		return models.VIPStatus{}
	}
	out := models.VIPStatus{
		Tier:                msg.GetTier(),
		VolumeTier:          msg.GetVolumeTier(),
		AopTier:             msg.GetAopTier(),
		SettledVolume30DUsd: cloneOptionalString(msg.SettledVolume_30DUsd),
		AverageAop30DUsd:    cloneOptionalString(msg.AverageAop_30DUsd),
		PolicyVersion:       msg.GetPolicyVersion(),
		PolicyEffectiveFrom: timestampTime(msg.GetPolicyEffectiveFrom()),
		EffectiveFrom:       timestampTime(msg.EffectiveFrom),
		EvaluatedAt:         timestampTime(msg.EvaluatedAt),
		MetricsAsOf:         timestampTime(msg.MetricsAsOf),
	}
	if next := msg.NextTierThresholds; next != nil {
		decoded := NextVIPTierThresholdsFromProto(next)
		out.NextTierThresholds = &decoded
	}
	return out
}
