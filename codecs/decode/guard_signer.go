package decode

import (
	"encoding/hex"

	guardv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/guard/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func CreateWalletFromProto(msg *guardv1.CreateGuardSignerWalletResponse) models.CreateGuardSignerWalletResult {
	return models.CreateGuardSignerWalletResult{SignerWallet: msg.GetSignerAddress()}
}

func StatusFromProto(msg *guardv1.GetGuardSignerStatusResponse) *models.GuardSignerStatus {
	if msg.GetStatus() == nil {
		return nil
	}
	st := msg.GetStatus()
	status := "initialized"
	if !st.GetInitialized() {
		status = "uninitialized"
	}
	return &models.GuardSignerStatus{SignerWallet: st.GetSignerAddress(), Status: status}
}

func SignProtectedActionFromProto(msg *guardv1.SignProtectedActionResponse) *models.GuardApproval {
	if msg.GetApproval() == nil {
		return nil
	}
	a := msg.GetApproval()
	return &models.GuardApproval{Signature: hex.EncodeToString(a.GetSignature())}
}

func BatchSignFromProto(msg *guardv1.BatchSignProtectedActionsResponse) models.BatchSignProtectedActionsResult {
	out := make([]models.GuardApproval, 0, len(msg.GetApprovals()))
	for _, a := range msg.GetApprovals() {
		out = append(out, models.GuardApproval{Signature: hex.EncodeToString(a.GetSignature())})
	}
	return models.BatchSignProtectedActionsResult{Approvals: out}
}

func RotateWalletFromProto(msg *guardv1.RotateGuardSignerWalletResponse) models.RotateGuardSignerWalletResult {
	return models.RotateGuardSignerWalletResult{SignerWallet: msg.GetNewSignerAddress()}
}

func ExportWalletFromProto(msg *guardv1.ExportGuardSignerWalletResponse) models.ExportGuardSignerWalletResult {
	return models.ExportGuardSignerWalletResult{EncryptedPrivateKey: msg.GetPrivateKey()}
}
