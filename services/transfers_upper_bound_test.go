package services

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ListTransfersRequest.ts_max_us has explicit presence: an explicit 0 asks for
// an empty page bounded at the epoch, while an omitted bound is unbounded.
// Transfers.List sets no upper bound, so on both wire formats the field is
// absent, never 0.
func TestTransfersListOmitsTheUpperBoundOnEveryWire(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		for _, since := range []*int64{nil, new(int64(0)), new(int64(1_793_922_052_911_123))} {
			t.Run(wire, func(t *testing.T) {
				bodies := make(chan []byte, 1)
				srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					bodies <- body
					var resp []byte
					if wire == "json" {
						w.Header().Set("Content-Type", "application/json")
						resp = []byte("{}")
					} else {
						w.Header().Set("Content-Type", "application/proto")
						resp, _ = proto.Marshal(&ledgerrdv1.ListTransfersResponse{})
					}
					_, _ = w.Write(resp)
				}))
				t.Cleanup(srv.Close)
				creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
				if err != nil {
					t.Fatal(err)
				}
				cfg := transport.DefaultConfig()
				cfg.APIURL = srv.URL
				cfg.WireFormat = transport.WireFormatFromString(wire)
				if _, err = NewTransfersService(transport.NewFactory(cfg, creds, nil), nil, nil, nil).List(context.Background(), nil, nil, 50, false, since); err != nil {
					t.Fatal(err)
				}
				body := <-bodies
				req := &ledgerrdv1.ListTransfersRequest{}
				if wire == "json" {
					if strings.Contains(string(body), "tsMaxUs") || strings.Contains(string(body), "ts_max_us") {
						t.Fatalf("JSON request carries an upper bound: %s", body)
					}
					err = protojson.Unmarshal(body, req)
				} else {
					err = proto.Unmarshal(body, req)
				}
				if err != nil {
					t.Fatal(err)
				}
				if req.TsMaxUs != nil {
					t.Fatalf("request carries ts_max_us=%d, want it absent", *req.TsMaxUs)
				}
			})
		}
	}
}
