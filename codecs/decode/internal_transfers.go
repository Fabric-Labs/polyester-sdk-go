package decode

import (
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func InternalTransferFromProto(msg *transferv1.CreateInternalTransferResponse) models.InternalTransferResult {
	aid := msg.GetAssetId()
	return models.InternalTransferResult{
		RequestID:  msg.GetRequestId(),
		TransferID: msg.GetTransferId(),
		AssetID:    aid,
		AssetCode:  msg.GetAssetCode(),
		Quantity: codecs.DecodeAssetAmount(
			msg.GetQtyScaled(),
			codecs.LedgerScale,
			models.QuantityDomainAsset,
			&aid,
		),
	}
}
