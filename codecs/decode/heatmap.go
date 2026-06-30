package decode

import (
	heatmapv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func HeatmapFromProto(msg *heatmapv1.GetOrderbookHeatmapResponse) models.ApiData {
	return APIDataFromProtoMust(msg)
}
