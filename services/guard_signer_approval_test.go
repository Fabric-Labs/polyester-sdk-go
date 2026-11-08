package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/connectx"
	guardv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/guard/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/guard/v1/chainguardv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// Values above 2^53 prove the SDK keeps the exact 64-bit approval fields.
const (
	approvalNonceSpace   uint64 = 18_446_744_073_709_551_615
	approvalDeadlineUnix uint64 = 9_007_199_254_740_993
)

type guardApprovalFixture struct {
	chainguardv1connect.UnimplementedGuardSignerServiceHandler
	rotateApproval bool
}

func fixtureApproval(sig byte) *guardv1.GuardApproval {
	return &guardv1.GuardApproval{NonceSpace: approvalNonceSpace, DeadlineUnix: approvalDeadlineUnix, Signature: []byte{0xab, sig}}
}

func (guardApprovalFixture) SignProtectedAction(context.Context, *connect.Request[guardv1.SignProtectedActionRequest]) (*connect.Response[guardv1.SignProtectedActionResponse], error) {
	return connect.NewResponse(&guardv1.SignProtectedActionResponse{Approval: fixtureApproval(0x01)}), nil
}

func (guardApprovalFixture) BatchSignProtectedActions(context.Context, *connect.Request[guardv1.BatchSignProtectedActionsRequest]) (*connect.Response[guardv1.BatchSignProtectedActionsResponse], error) {
	return connect.NewResponse(&guardv1.BatchSignProtectedActionsResponse{Approvals: []*guardv1.GuardApproval{
		fixtureApproval(0x02),
		{Signature: []byte{0xab, 0x03}},
	}}), nil
}

func (f guardApprovalFixture) RotateGuardSignerWallet(context.Context, *connect.Request[guardv1.RotateGuardSignerWalletRequest]) (*connect.Response[guardv1.RotateGuardSignerWalletResponse], error) {
	resp := &guardv1.RotateGuardSignerWalletResponse{NewSignerAddress: "0x2222222222222222222222222222222222222222"}
	if f.rotateApproval {
		resp.Approval = fixtureApproval(0x04)
	}
	return connect.NewResponse(resp), nil
}

func guardApprovalService(t *testing.T, wire connectx.WireFormat, rotateApproval bool) *GuardSignerService {
	t.Helper()
	path, handler := chainguardv1connect.NewGuardSignerServiceHandler(guardApprovalFixture{rotateApproval: rotateApproval})
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	creds, err := auth.LoadCredentials("ak_local", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = server.URL
	cfg.WireFormat = wire
	return NewGuardSignerService(transport.NewFactory(cfg, creds, server.Client()), nil)
}

func assertFixtureApproval(t *testing.T, got models.GuardApproval, sig string) {
	t.Helper()
	want := models.GuardApproval{Signature: sig, NonceSpace: approvalNonceSpace, DeadlineUnix: approvalDeadlineUnix}
	if got != want {
		t.Fatalf("approval=%+v want %+v", got, want)
	}
}

func TestGuardSignerApprovalKeepsNonceAndDeadline(t *testing.T) {
	for _, wire := range []connectx.WireFormat{connectx.WireBinary, connectx.WireJSON} {
		t.Run(string(wire), func(t *testing.T) {
			ctx := context.Background()
			guard := guardApprovalService(t, wire, true)

			single, err := guard.SignProtectedAction(ctx, "withdraw", nil, nil, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if single == nil {
				t.Fatal("approval missing")
			}
			assertFixtureApproval(t, *single, "ab01")

			batch, err := guard.BatchSignProtectedActions(ctx, []map[string]any{{"action": "withdraw"}, {"action": "withdraw"}}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(batch.Approvals) != 2 {
				t.Fatalf("approvals=%+v", batch.Approvals)
			}
			assertFixtureApproval(t, batch.Approvals[0], "ab02")
			if got := batch.Approvals[1]; got != (models.GuardApproval{Signature: "ab03"}) {
				t.Fatalf("zero-valued approval=%+v", got)
			}

			rotated, err := guard.RotateWallet(ctx, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if rotated.SignerWallet != "0x2222222222222222222222222222222222222222" {
				t.Fatalf("signer=%q", rotated.SignerWallet)
			}
			if rotated.Approval == nil {
				t.Fatal("rotation approval dropped")
			}
			assertFixtureApproval(t, *rotated.Approval, "ab04")
		})
	}
}

func TestGuardSignerRotationWithoutApproval(t *testing.T) {
	for _, wire := range []connectx.WireFormat{connectx.WireBinary, connectx.WireJSON} {
		t.Run(string(wire), func(t *testing.T) {
			rotated, err := guardApprovalService(t, wire, false).RotateWallet(context.Background(), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if rotated.Approval != nil {
				t.Fatalf("absent approval decoded as %+v", rotated.Approval)
			}
		})
	}
}
