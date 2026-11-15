package decode

import (
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
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
