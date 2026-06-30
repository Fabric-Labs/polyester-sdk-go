package decode

import (
	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func WithdrawIntentFromProto(msg *chainwithdrawv1.CreateTradingWithdrawResponse) models.WithdrawIntentResult {
	return models.WithdrawIntentResult{IntentID: msg.GetIntentId()}
}

func WithdrawIntentFromWalletProto(msg *chainwithdrawv1.CreateWalletTradingWithdrawResponse) models.WithdrawIntentResult {
	return models.WithdrawIntentResult{IntentID: msg.GetIntentId()}
}
