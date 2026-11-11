package decode

import (
	"fmt"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
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

// withdrawDestinationValidationCodeLabel maps validation codes to snake labels
// matching the TypeScript SDK. Unknown codes become unknown_code_<n>.
func withdrawDestinationValidationCodeLabel(code chainwithdrawv1.WithdrawDestinationValidationCode) string {
	switch code {
	case chainwithdrawv1.WithdrawDestinationValidationCode_RESULT_UNSPECIFIED:
		return "unspecified"
	case chainwithdrawv1.WithdrawDestinationValidationCode_VALID:
		return "valid"
	case chainwithdrawv1.WithdrawDestinationValidationCode_INVALID_ADDRESS:
		return "invalid_address"
	case chainwithdrawv1.WithdrawDestinationValidationCode_UNSUPPORTED_CHAIN:
		return "unsupported_chain"
	case chainwithdrawv1.WithdrawDestinationValidationCode_POLYESTER_SMART_ACCOUNT:
		return "polyester_smart_account"
	case chainwithdrawv1.WithdrawDestinationValidationCode_TOKEN_CONTRACT:
		return "token_contract"
	case chainwithdrawv1.WithdrawDestinationValidationCode_DENYLISTED_ADDRESS:
		return "denylisted_address"
	default:
		return fmt.Sprintf("unknown_code_%d", int32(code))
	}
}

func WithdrawDestinationValidationFromProto(msg *chainwithdrawv1.ValidateWithdrawDestinationResponse) models.WithdrawDestinationValidation {
	if msg == nil {
		return models.WithdrawDestinationValidation{Code: "unspecified"}
	}
	return models.WithdrawDestinationValidation{
		Valid:                       msg.GetValid(),
		Code:                        withdrawDestinationValidationCodeLabel(msg.GetCode()),
		Message:                     msg.GetMessage(),
		CanonicalDestinationAddress: msg.GetCanonicalDestinationAddress(),
	}
}

// WalletBalancesFromProto decodes the smart account wallet's zToken balances.
func WalletBalancesFromProto(msg *chainwithdrawv1.GetWalletBalancesResponse) (models.WalletBalances, error) {
	out := models.WalletBalances{
		SmartAccountAddress: msg.GetSmartAccountAddress(),
		Balances:            make([]models.WalletBalance, 0, len(msg.GetBalances())),
	}
	for _, b := range msg.GetBalances() {
		raw := u128(b.GetAmountE18())
		amount, err := codecs.FormatLedgerU128(raw, codecs.LedgerScale)
		if err != nil {
			return models.WalletBalances{}, &sdkerrors.TransportError{Msg: fmt.Sprintf("wallet balance of zipped asset %d: %v", b.GetZippedAssetId(), err)}
		}
		out.Balances = append(out.Balances, models.WalletBalance{
			ZippedAssetID:  b.GetZippedAssetId(),
			UnifiedAssetID: b.GetUnifiedAssetId(),
			ChainID:        b.GetChainId(),
			ZTokenAddress:  b.GetZtokenAddress(),
			AmountE18:      raw,
			Amount:         amount,
		})
	}
	return out, nil
}
