package decode

import (
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	analyticsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/analytics/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
	"google.golang.org/protobuf/proto"
)

// APIDataFromProto wraps any proto response as ApiData.
func APIDataFromProto(msg proto.Message) (models.ApiData, error) {
	raw, err := wire.ProtoToMap(msg)
	if err != nil {
		return models.ApiData{}, err
	}
	return models.ApiData{Raw: raw}, nil
}

// APIDataFromProtoMust wraps proto as ApiData and panics on error (for decoders).
func APIDataFromProtoMust(msg proto.Message) models.ApiData {
	data, err := APIDataFromProto(msg)
	if err != nil {
		return models.ApiData{Raw: map[string]any{}}
	}
	return data
}

// requireID refuses a reply whose required public ID is 0, the proto default
// of an absent field: it breaks the response contract, and the call may have
// applied, so the caller must not keep "0" as the ID of what it changed.
func requireID(operation, field string, id uint64) error {
	if id == 0 {
		return &sdkerrors.ResponseContractError{Operation: operation, Msg: "missing " + field}
	}
	return nil
}

// requireStringID refuses a reply whose required string ID is empty, as
// requireID refuses 0.
func requireStringID(operation, field, id string) error {
	if id == "" {
		return &sdkerrors.ResponseContractError{Operation: operation, Msg: "missing " + field}
	}
	return nil
}

// ZippedAssetSupplyGroupFromProto wraps a GetZippedAssetSupplyGroup reply as
// ApiData. A reply without its group ID returns a
// *errors.ResponseContractError.
func ZippedAssetSupplyGroupFromProto(msg *analyticsv1.GetZippedAssetSupplyGroupResponse) (models.ApiData, error) {
	if err := requireStringID("GetZippedAssetSupplyGroup", "group_id", msg.GetGroupId()); err != nil {
		return models.ApiData{}, err
	}
	return APIDataFromProtoMust(msg), nil
}
