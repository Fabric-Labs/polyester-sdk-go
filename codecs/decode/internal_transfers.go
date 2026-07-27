package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func InternalTransferFromProto(msg *transferv1.CreateInternalTransferResponse) (models.InternalTransferResult, error) {
	if msg.GetRequestId() == "" || msg.GetTransferId() == "" {
		return models.InternalTransferResult{}, &sdkerrors.ResponseContractError{Operation: "CreateInternalTransfer", Msg: "missing request_id or transfer_id"}
	}
	aid := msg.GetAssetId()
	amount := msg.GetAmountE18()
	var scaled = codecs.U128ToBig(0, 0)
	if amount != nil {
		scaled = codecs.U128ToBig(amount.GetHi(), amount.GetLo())
	}
	return models.InternalTransferResult{
		RequestID:  msg.GetRequestId(),
		TransferID: msg.GetTransferId(),
		AssetID:    aid,
		AssetCode:  msg.GetAssetCode(),
		Quantity: codecs.DecodeAssetAmountBig(
			scaled,
			codecs.LedgerScale,
			models.QuantityDomainLedgerE18,
			&aid,
		),
	}, nil
}
