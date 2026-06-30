package services

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

func apiData(msg proto.Message) models.ApiData {
	return decode.APIDataFromProtoMust(msg)
}
