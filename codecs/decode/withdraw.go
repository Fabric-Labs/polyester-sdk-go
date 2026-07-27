package decode

import (
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func WithdrawIntentFromProto(msg *chainwithdrawv1.CreateTradingWithdrawResponse) (models.WithdrawIntentResult, error) {
	if msg.GetIntentId() == "" {
		return models.WithdrawIntentResult{}, &sdkerrors.ResponseContractError{Operation: "CreateTradingWithdraw", Msg: "missing intent_id"}
	}
	return models.WithdrawIntentResult{IntentID: msg.GetIntentId()}, nil
}

func WithdrawIntentFromWalletProto(msg *chainwithdrawv1.CreateWalletTradingWithdrawResponse) (models.WithdrawIntentResult, error) {
	if msg.GetIntentId() == "" {
		return models.WithdrawIntentResult{}, &sdkerrors.ResponseContractError{Operation: "CreateWalletTradingWithdraw", Msg: "missing intent_id"}
	}
	return models.WithdrawIntentResult{IntentID: msg.GetIntentId()}, nil
}
