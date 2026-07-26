package codecs

import (
	"strings"

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

func ProtectedActionArgsToProto(externalChainID *int, external, internal []string, whitelistRequired *bool) *guardv1.ProtectedActionArgs {
	args := &guardv1.ProtectedActionArgs{}
	if externalChainID != nil || len(external) > 0 {
		ext := &guardv1.ExternalWhitelistArgs{Addresses: append([]string(nil), external...)}
		if externalChainID != nil {
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
	return args
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
	if v, ok := item["action"].(string); ok {
		out.Action = v
	}
	if v, ok := item["external_polychain_chain_id"]; ok && v != nil {
		switch n := v.(type) {
		case int:
			out.ExternalPolychainChainID = &n
		case int32:
			i := int(n)
			out.ExternalPolychainChainID = &i
		case int64:
			i := int(n)
			out.ExternalPolychainChainID = &i
		case float64:
			i := int(n)
			out.ExternalPolychainChainID = &i
		}
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
func BatchProtectedActionItemToProto(item BatchProtectedActionInput) *guardv1.BatchSignProtectedActionItem {
	return &guardv1.BatchSignProtectedActionItem{
		Action: ProtectedActionFromLabel(item.Action),
		Args: ProtectedActionArgsToProto(
			item.ExternalPolychainChainID,
			item.ExternalAddresses,
			item.InternalAddresses,
			item.WhitelistRequired,
		),
	}
}
