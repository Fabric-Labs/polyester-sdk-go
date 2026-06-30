package decode

import (
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
