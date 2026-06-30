package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func ResolvedAccountsFromProto(msg *authv1.ResolveAccountResponse) models.ResolvedAccountsList {
	accounts := make([]models.ResolvedAccount, 0, len(msg.GetMatches()))
	for _, a := range msg.GetMatches() {
		username := a.GetRootUsername()
		if a.GetSubaccountLabel() != "" {
			username = a.GetSubaccountLabel()
		}
		accounts = append(accounts, models.ResolvedAccount{
			AccountID:           codecs.FormatUint64ID(a.GetAccountId()),
			Username:            username,
			SmartAccountAddress: a.GetSmartAccountAddress(),
		})
	}
	return models.ResolvedAccountsList{Accounts: accounts}
}
