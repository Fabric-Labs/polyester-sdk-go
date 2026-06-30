package decode

import (
	chaindepositv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/deposit/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func depositAddress(msg *chaindepositv1.DepositAddress) models.DepositAddress {
	if msg == nil {
		return models.DepositAddress{}
	}
	return models.DepositAddress{ChainID: msg.GetChainId(), DepositAddress: msg.GetDepositAddress()}
}

func DepositAddressesListFromProto(msg *chaindepositv1.ListDepositAddressesResponse) models.DepositAddressesList {
	out := make([]models.DepositAddress, 0, len(msg.GetDepositAddresses()))
	for _, a := range msg.GetDepositAddresses() {
		out = append(out, depositAddress(a))
	}
	return models.DepositAddressesList{Addresses: out}
}

func CreateDepositAddressFromProto(msg *chaindepositv1.CreateDepositAddressResponse) models.DepositAddress {
	return depositAddress(msg.GetDepositAddress())
}
