package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// A ledger read refused with ledger.read.v1.ErrorDetail reports its stable
// code, known and final, through the public balances read.
func TestBalancesReadReportsTheLedgerErrorCode(t *testing.T) {
	raw, err := proto.Marshal(&ledgerrdv1.ErrorDetail{Code: ledgerrdv1.ErrorCode_ERROR_CODE_INVALID_SUBACCOUNT_ID})
	if err != nil {
		t.Fatal(err)
	}
	if ledgerrdv1.ErrorCode_ERROR_CODE_INVALID_SUBACCOUNT_ID != 11 {
		t.Fatal("the contract numbers this code 11")
	}
	body, err := json.Marshal(map[string]any{
		"code":    "invalid_argument",
		"message": "subaccount_id is not valid",
		"details": []map[string]string{{
			"type":  "ledger.read.v1.ErrorDetail",
			"value": base64.RawStdEncoding.EncodeToString(raw),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/ledger.read.v1.LedgerReadService/") {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	service := NewBalancesService(transport.NewFactory(cfg, creds, nil), nil, nil, nil, nil)
	_, err = service.List(context.Background(), nil, nil)
	if calls.Load() != 1 {
		t.Fatalf("requests=%d err=%v", calls.Load(), err)
	}
	var apiErr *sdkerrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "ERROR_CODE_INVALID_SUBACCOUNT_ID" {
		t.Fatalf("got %T %+v", err, err)
	}
	if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("a refused read is known and final: %+v", err)
	}
}
