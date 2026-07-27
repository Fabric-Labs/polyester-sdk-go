package decode

import (
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
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

func CreateDepositAddressFromProto(msg *chaindepositv1.CreateDepositAddressResponse) (models.DepositAddress, error) {
	if msg.GetDepositAddress() == nil {
		return models.DepositAddress{}, &sdkerrors.TransportError{
			Msg: "invalid CreateDepositAddress response: missing deposit_address",
		}
	}
	result := depositAddress(msg.GetDepositAddress())
	if result.DepositAddress == "" {
		return models.DepositAddress{}, &sdkerrors.TransportError{
			Msg: "invalid CreateDepositAddress response: empty deposit address",
		}
	}
	return result, nil
}
