package decode

import (
	"strconv"

	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func InternalTransferFromProto(msg *transferv1.CreateInternalTransferResponse) models.InternalTransferResult {
	return models.InternalTransferResult{
		RequestID: msg.GetRequestId(), TransferID: msg.GetTransferId(),
		AssetID: msg.GetAssetId(), AssetCode: msg.GetAssetCode(), QuantityScaled: strconv.FormatInt(msg.GetQtyScaled(), 10),
	}
}
