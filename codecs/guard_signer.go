package codecs

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	guardv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/guard/v1"
)

func ProtectedActionFromLabel(label string) guardv1.ProtectedAction {
	name := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(label), "-", "_"))
	if v, ok := guardv1.ProtectedAction_value[name]; ok {
		return guardv1.ProtectedAction(v)
	}
	if v, ok := guardv1.ProtectedAction_value["PROTECTED_ACTION_"+name]; ok {
		return guardv1.ProtectedAction(v)
	}
	return guardv1.ProtectedAction_PROTECTED_ACTION_UNSPECIFIED
}

// ProtectedActionArgsToProto encodes protected-action arguments. A chain ID
// outside the wire uint32 range is refused, never wrapped to another chain.
func ProtectedActionArgsToProto(externalChainID *int, external, internal []string, whitelistRequired *bool) (*guardv1.ProtectedActionArgs, error) {
	args := &guardv1.ProtectedActionArgs{}
	if externalChainID != nil || len(external) > 0 {
		ext := &guardv1.ExternalWhitelistArgs{Addresses: append([]string(nil), external...)}
		if externalChainID != nil {
			if *externalChainID < 0 || uint64(*externalChainID) > math.MaxUint32 {
				return nil, chainIDRangeError(fmt.Sprint(*externalChainID))
			}
			ext.PolychainChainId = uint32(*externalChainID)
		}
		args.Args = &guardv1.ProtectedActionArgs_ExternalWhitelist{ExternalWhitelist: ext}
	} else if len(internal) > 0 {
		args.Args = &guardv1.ProtectedActionArgs_InternalWhitelist{
			InternalWhitelist: &guardv1.InternalWhitelistArgs{Addresses: append([]string(nil), internal...)},
		}
	} else if whitelistRequired != nil {
		args.Args = &guardv1.ProtectedActionArgs_WhitelistRequirement{
			WhitelistRequirement: &guardv1.WhitelistRequirementArgs{Required: *whitelistRequired},
		}
	}
	return args, nil
}

func chainIDRangeError(value string) error {
	return &errors.ValidationError{Msg: fmt.Sprintf("external_polychain_chain_id %s is outside 0..%d", value, uint32(math.MaxUint32))}
}

// chainIDFromAny reads a batch item's chain ID: a whole number from 0 to
// MaxUint32 of any Go integer or float type (JSON numbers decode as
// float64), or a json.Number. Anything else is refused, not dropped,
// truncated or wrapped.
func chainIDFromAny(v any) (int, error) {
	var (
		n  uint64
		ok bool
	)
	switch t := v.(type) {
	case int:
		n, ok = uint64(t), t >= 0
	case int8:
		n, ok = uint64(t), t >= 0
	case int16:
		n, ok = uint64(t), t >= 0
	case int32:
		n, ok = uint64(t), t >= 0
	case int64:
		n, ok = uint64(t), t >= 0
	case uint:
		n, ok = uint64(t), true
	case uint8:
		n, ok = uint64(t), true
	case uint16:
		n, ok = uint64(t), true
	case uint32:
		n, ok = uint64(t), true
	case uint64:
		n, ok = t, true
	case float32:
		return chainIDFromFloat(float64(t), v)
	case float64:
		return chainIDFromFloat(t, v)
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0, &errors.ValidationError{Msg: fmt.Sprintf("external_polychain_chain_id %s must be a whole number", t)}
		}
		n, ok = uint64(i), i >= 0
	default:
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("external_polychain_chain_id must be a number, got %T", v)}
	}
	if !ok || n > math.MaxUint32 {
		return 0, chainIDRangeError(fmt.Sprint(v))
	}
	return int(n), nil
}

func chainIDFromFloat(f float64, original any) (int, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("external_polychain_chain_id %v must be a whole number", original)}
	}
	if f < 0 || f > math.MaxUint32 {
		return 0, chainIDRangeError(fmt.Sprint(original))
	}
	return int(f), nil
}

// BatchProtectedActionInput is one item for batch protected-action signing.
type BatchProtectedActionInput struct {
	Action                   string
	ExternalPolychainChainID *int
	ExternalAddresses        []string
	InternalAddresses        []string
	WhitelistRequired        *bool
}

// BatchProtectedActionFromMap parses one batch-sign action item.
func BatchProtectedActionFromMap(item map[string]any) (BatchProtectedActionInput, error) {
	out := BatchProtectedActionInput{}
	if item == nil {
		return out, nil
	}
	for _, key := range []string{"sub_account_id", "subaccount_id", "subaccountId", "account"} {
		if value, ok := item[key]; ok && value != nil {
			return out, &errors.ValidationError{Msg: "batch sign action scope is unsupported; use the batch-level subAccountID"}
		}
	}
	if v, ok := item["action"].(string); ok {
		out.Action = v
	}
	if v, ok := item["external_polychain_chain_id"]; ok && v != nil {
		chainID, err := chainIDFromAny(v)
		if err != nil {
			return out, err
		}
		out.ExternalPolychainChainID = &chainID
	}
	out.ExternalAddresses = stringSliceFromAny(item["external_addresses"])
	out.InternalAddresses = stringSliceFromAny(item["internal_addresses"])
	if v, ok := item["whitelist_required"].(bool); ok {
		out.WhitelistRequired = &v
	}
	return out, nil
}

func stringSliceFromAny(v any) []string {
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// BatchProtectedActionItemToProto converts a batch input to proto.
func BatchProtectedActionItemToProto(item BatchProtectedActionInput) (*guardv1.BatchSignProtectedActionItem, error) {
	args, err := ProtectedActionArgsToProto(
		item.ExternalPolychainChainID,
		item.ExternalAddresses,
		item.InternalAddresses,
		item.WhitelistRequired,
	)
	if err != nil {
		return nil, err
	}
	return &guardv1.BatchSignProtectedActionItem{
		Action: ProtectedActionFromLabel(item.Action),
		Args:   args,
	}, nil
}
