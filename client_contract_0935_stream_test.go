package polyester

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// supplyStreamClient is a client whose funding catalog loads from config at
// construction, as a real client's does, and whose realtime connection
// serves one supply publication once subscribed.
func supplyStreamClient(t *testing.T, config *zipperv1.GetDepositWithdrawConfigResponse, publication proto.Message) *Client {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := chainzipperv1connect.NewZipperServiceHandler(&fakeZipperConfig{config: config})
	mux.Handle(path, handler)
	api := hardening.NewTestServer(mux)
	t.Cleanup(api.Close)

	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
	ws := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		// Answer the connect (id 1) and the subscribe (id 2), then publish.
		for _, ack := range [][]byte{{2, 8, 1}, {2, 8, 2}} {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, ack); err != nil {
				return
			}
		}
		data, err := proto.Marshal(publication)
		if err != nil {
			return
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, publicationPush(data)); err != nil {
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(ws.Close)

	keypair := auth.GenerateEd25519Keypair()
	client, err := New(Config{
		APIURL:        api.URL,
		WSURL:         "ws" + strings.TrimPrefix(ws.URL, "http"),
		APIKeyID:      "ak_supply",
		APIPrivateKey: keypair.SecretKeyHex,
		Timeout:       5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// zipperChain returns the catalog's chain of the zipped asset id.
func zipperChain(t *testing.T, client *Client, zippedAssetID uint32) models.ZipperEnrichedAssetChain {
	t.Helper()
	catalog := client.Catalogs.Zipper
	if catalog == nil {
		t.Fatal("no funding catalog")
	}
	for _, asset := range catalog.Assets {
		for _, chain := range asset.Chains {
			if chain.ZippedAssetID == zippedAssetID {
				return chain
			}
		}
	}
	t.Fatalf("funding catalog lacks zipped asset %d", zippedAssetID)
	return models.ZipperEnrichedAssetChain{}
}

// publicationPush frames data as one Centrifugo protobuf push publication.
func publicationPush(data []byte) []byte {
	pub := protowire.AppendTag(nil, 4, protowire.BytesType)
	pub = protowire.AppendBytes(pub, data)
	push := protowire.AppendTag(nil, 4, protowire.BytesType)
	push = protowire.AppendBytes(push, pub)
	reply := protowire.AppendTag(nil, 4, protowire.BytesType)
	reply = protowire.AppendBytes(reply, push)
	return append(protowire.AppendVarint(nil, uint64(len(reply))), reply...)
}

// A supply stream update with a known supply but no read time ends the
// subscription with the typed *errors.ResponseContractError, kept behind the
// *errors.RealtimeError rather than flattened to its text. No batch is
// delivered and the catalog is not patched.
func TestSupplyStreamEndsWithTheTypedContractError(t *testing.T) {
	for _, status := range []zipperv1.SupplyStatus{zipperv1.SupplyStatus_AVAILABLE, zipperv1.SupplyStatus_STALE} {
		t.Run(status.String(), func(t *testing.T) {
			client := supplyStreamClient(t, supplyConfig(zipperv1.SupplyStatus_MISSING, nil),
				&zipperv1.ZippedAssetSupplyBatch{Updates: []*zipperv1.ZippedAssetSupplyUpdate{{ZippedAssetId: 3, SupplyQ: 7, Status: status}}})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.WaitForFundingCatalog(ctx); err != nil {
				t.Fatal(err)
			}
			sub, err := client.Zipper.SubscribeZippedAssetSupply(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			select {
			case <-sub.Done():
			case <-ctx.Done():
				t.Fatal("the stream did not end on an update that breaks the contract")
			}
			var contract *sdkerrors.ResponseContractError
			var realtimeErr *sdkerrors.RealtimeError
			if err := sub.Err(); !stderrors.As(err, &contract) || !stderrors.As(err, &realtimeErr) || !strings.Contains(err.Error(), "observed_at") {
				t.Fatalf("Err() = %T %v, want a RealtimeError wrapping a ResponseContractError", err, err)
			}
			select {
			case batch, ok := <-sub.Messages():
				if ok {
					t.Fatalf("delivered %+v from an update that breaks the contract", batch)
				}
			default:
			}
			if chain := zipperChain(t, client, 3); chain.Supply != "" || chain.SupplyStatus != "missing" || chain.SupplyObservedAt != nil {
				t.Fatalf("catalog patched from an update that breaks the contract: %+v", chain)
			}
		})
	}
	// A complete update is delivered.
	client := supplyStreamClient(t, supplyConfig(zipperv1.SupplyStatus_MISSING, nil),
		&zipperv1.ZippedAssetSupplyBatch{Updates: []*zipperv1.ZippedAssetSupplyUpdate{{ZippedAssetId: 3, SupplyQ: 1_500_000, Status: zipperv1.SupplyStatus_AVAILABLE, ObservedAt: timestamppb.New(observedAt)}}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.WaitForFundingCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	sub, err := client.Zipper.SubscribeZippedAssetSupply(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	select {
	case batch := <-sub.Messages():
		if len(batch.Updates) != 1 || batch.Updates[0].Supply != "1.5" || batch.Updates[0].Status != "available" || !batch.Updates[0].ObservedAt.Equal(observedAt) {
			t.Fatalf("batch = %+v", batch)
		}
	case <-ctx.Done():
		t.Fatalf("no update delivered: %v", sub.Err())
	}
}

// The funding catalog a client loads at construction refuses a config whose
// known supply has no read time: WaitForFundingCatalog returns a
// *errors.CatalogHydrationError wrapping the *errors.ResponseContractError,
// and no funding catalog is installed.
func TestFundingCatalogRefusesAKnownSupplyWithoutItsReadTime(t *testing.T) {
	for _, status := range []zipperv1.SupplyStatus{zipperv1.SupplyStatus_AVAILABLE, zipperv1.SupplyStatus_STALE} {
		client := supplyStreamClient(t, supplyConfig(status, nil), &zipperv1.ZippedAssetSupplyBatch{})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := client.WaitForFundingCatalog(ctx)
		cancel()
		var hydration *sdkerrors.CatalogHydrationError
		var contract *sdkerrors.ResponseContractError
		if !stderrors.As(err, &hydration) || !stderrors.As(err, &contract) {
			t.Fatalf("%s: WaitForFundingCatalog = %T %v, want a CatalogHydrationError wrapping a ResponseContractError", status, err, err)
		}
		if client.Catalogs.Zipper != nil || len(client.Catalogs.ZipperConfig()) != 0 {
			t.Fatalf("%s: a funding catalog was installed from a config that breaks the contract", status)
		}
	}
	client := supplyStreamClient(t, supplyConfig(zipperv1.SupplyStatus_AVAILABLE, timestamppb.New(observedAt)), &zipperv1.ZippedAssetSupplyBatch{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.WaitForFundingCatalog(ctx); err != nil {
		t.Fatalf("a complete config: %v", err)
	}
}
