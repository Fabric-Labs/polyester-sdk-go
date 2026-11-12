package services

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// API-key reads keep the contract's icon, color and created_by_actor, read a
// main-account key's SubaccountID as empty (unset or an explicit 0), and
// SubAccounts.Get returns the sub-account's API keys when asked for them.
// Every read goes through the public service over both wire formats; the
// streams decode through the same decoder.

const displaySubaccount = uint64(42)

func displayKeys() []*authv1.ApiKey {
	zero := uint64(0)
	sub := displaySubaccount
	return []*authv1.ApiKey{
		{
			KeyId: "ak_11111111111111111111111111111111", Label: "bot",
			Icon: "🤖", Color: "violet-500", CreatedByActor: "alice",
			Status: authv1.ApiKeyStatus_ACTIVE, Revision: 3,
		},
		// subaccount_id has presence: a server may send an explicit 0.
		{KeyId: "ak_22222222222222222222222222222222", Label: "zero", SubaccountId: &zero},
		{
			KeyId: "ak_33333333333333333333333333333333", Label: "desk", SubaccountId: &sub,
			Icon: "#", CreatedByActor: "desk-admin",
		},
	}
}

type displayWant struct {
	keyID, subaccount, icon, color, actor string
}

var displayWants = []displayWant{
	{"ak_11111111111111111111111111111111", "", "🤖", "violet-500", "alice"},
	{"ak_22222222222222222222222222222222", "", "", "", ""},
	{"ak_33333333333333333333333333333333", codecs.FormatID(displaySubaccount), "#", "", "desk-admin"},
}

func requireDisplayKeys(t *testing.T, label string, keys []models.ApiKeySummary) {
	t.Helper()
	if len(keys) != len(displayWants) {
		t.Fatalf("%s: %d keys, want %d: %+v", label, len(keys), len(displayWants), keys)
	}
	for i, want := range displayWants {
		k := keys[i]
		got := displayWant{k.KeyID, k.SubaccountID, k.Icon, k.Color, k.CreatedByActor}
		if got != want {
			t.Errorf("%s[%d] = %+v, want %+v", label, i, got, want)
		}
	}
}

type displayHandler struct {
	authv1connect.UnimplementedApiKeyServiceHandler
	authv1connect.UnimplementedSubaccountViewServiceHandler
}

func (displayHandler) ListApiKeys(context.Context, *connect.Request[authv1.ListApiKeysRequest]) (*connect.Response[authv1.ListApiKeysResponse], error) {
	return connect.NewResponse(&authv1.ListApiKeysResponse{ApiKeys: displayKeys()}), nil
}

func (displayHandler) GetApiKey(_ context.Context, req *connect.Request[authv1.GetApiKeyRequest]) (*connect.Response[authv1.GetApiKeyResponse], error) {
	for _, k := range displayKeys() {
		if k.GetKeyId() == req.Msg.GetKeyId() {
			return connect.NewResponse(&authv1.GetApiKeyResponse{ApiKey: k}), nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, nil)
}

// GetSubaccount answers API keys only when they were asked for, as the
// server does.
func (displayHandler) GetSubaccount(_ context.Context, req *connect.Request[authv1.GetSubaccountRequest]) (*connect.Response[authv1.GetSubaccountResponse], error) {
	resp := &authv1.GetSubaccountResponse{Subaccount: &authv1.Subaccount{Id: req.Msg.GetSubaccountId()}}
	if req.Msg.GetIncludeApiKeys() {
		resp.ApiKeys = displayKeys()
	}
	return connect.NewResponse(resp), nil
}

func newDisplayFactory(t *testing.T, wire string) *transport.Factory {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := authv1connect.NewApiKeyServiceHandler(displayHandler{})
	mux.Handle(path, handler)
	path, handler = authv1connect.NewSubaccountViewServiceHandler(displayHandler{})
	mux.Handle(path, handler)
	server := hardening.NewTestServer(mux)
	t.Cleanup(server.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = server.URL
	cfg.WireFormat = transport.WireFormatFromString(wire)
	return transport.NewFactory(cfg, creds, nil)
}

func TestApiKeyReadsKeepDisplayFieldsAndMainAccount(t *testing.T) {
	for _, wire := range retentionWires {
		t.Run(wire, func(t *testing.T) {
			svc := NewApiKeysService(newDisplayFactory(t, wire), nil, nil, nil)
			ctx := context.Background()

			listed, err := svc.List(ctx, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			requireDisplayKeys(t, "List", listed.Keys)

			got := make([]models.ApiKeySummary, 0, len(displayWants))
			for _, want := range displayWants {
				key, err := svc.Get(ctx, want.keyID)
				if err != nil {
					t.Fatal(err)
				}
				if key == nil {
					t.Fatalf("Get(%s): key missing", want.keyID)
				}
				got = append(got, *key)
			}
			requireDisplayKeys(t, "Get", got)
		})
	}
}

func TestSubAccountsGetReturnsAPIKeysWhenAsked(t *testing.T) {
	for _, wire := range retentionWires {
		t.Run(wire, func(t *testing.T) {
			svc := NewSubAccountsService(newDisplayFactory(t, wire), nil, nil, nil)
			ctx := context.Background()
			subID := codecs.FormatID(displaySubaccount)

			plain, err := svc.Get(ctx, nil, &subID, false, false, false, false, false, "")
			if err != nil {
				t.Fatal(err)
			}
			if plain.APIKeys != nil {
				t.Fatalf("APIKeys without includeAPIKeys = %+v", plain.APIKeys)
			}

			withKeys, err := svc.Get(ctx, nil, &subID, true, false, false, false, false, "")
			if err != nil {
				t.Fatal(err)
			}
			if withKeys.Subaccount == nil || withKeys.Subaccount.SubaccountID != subID {
				t.Fatalf("Subaccount = %+v", withKeys.Subaccount)
			}
			requireDisplayKeys(t, "SubAccounts.Get", withKeys.APIKeys)

			// The same decoder as APIKeys.List: every contract field is kept.
			listed := decode.ApiKeysListFromProto(&authv1.ListApiKeysResponse{ApiKeys: displayKeys()})
			if !reflect.DeepEqual(withKeys.APIKeys, listed.Keys) {
				t.Fatalf("SubAccounts.Get keys %+v != List keys %+v", withKeys.APIKeys, listed.Keys)
			}
		})
	}
}

func TestApiKeyStreamKeepsDisplayFieldsAndMainAccount(t *testing.T) {
	got := make([]models.ApiKeySummary, 0, len(displayWants))
	for _, k := range displayKeys() {
		payload, err := proto.Marshal(k)
		if err != nil {
			t.Fatal(err)
		}
		row, err := decode.ApiKeyFromBytes(payload)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	requireDisplayKeys(t, "stream", got)
}
