package polyester

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1/ledgerrdv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeStatementExports struct {
	ledgerrdv1connect.UnimplementedStatementExportServiceHandler

	mu       sync.Mutex
	creates  []*ledgerrdv1.CreateStatementExportRequest
	cancels  []*ledgerrdv1.CancelStatementExportRequest
	urls     []*ledgerrdv1.GetStatementExportDownloadUrlRequest
	unsigned int
	cancel   *ledgerrdv1.CancelStatementExportResponse
	url      *ledgerrdv1.GetStatementExportDownloadUrlResponse
	list     *ledgerrdv1.ListStatementExportsResponse
}

func (f *fakeStatementExports) signed(h http.Header) {
	if h.Get("X-API-KEY-ID") == "" || h.Get("X-API-SIGNATURE") == "" {
		f.unsigned++
	}
}

func (f *fakeStatementExports) CreateStatementExport(_ context.Context, req *connect.Request[ledgerrdv1.CreateStatementExportRequest]) (*connect.Response[ledgerrdv1.CreateStatementExportResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signed(req.Header())
	f.creates = append(f.creates, proto.Clone(req.Msg).(*ledgerrdv1.CreateStatementExportRequest))
	return connect.NewResponse(&ledgerrdv1.CreateStatementExportResponse{Export: &ledgerrdv1.StatementExport{
		ExportId: 7000000001,
		Status:   ledgerrdv1.StatementExportStatus_STATEMENT_EXPORT_STATUS_QUEUED,
	}}), nil
}

func (f *fakeStatementExports) CancelStatementExport(_ context.Context, req *connect.Request[ledgerrdv1.CancelStatementExportRequest]) (*connect.Response[ledgerrdv1.CancelStatementExportResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signed(req.Header())
	f.cancels = append(f.cancels, proto.Clone(req.Msg).(*ledgerrdv1.CancelStatementExportRequest))
	return connect.NewResponse(f.cancel), nil
}

func (f *fakeStatementExports) GetStatementExportDownloadUrl(_ context.Context, req *connect.Request[ledgerrdv1.GetStatementExportDownloadUrlRequest]) (*connect.Response[ledgerrdv1.GetStatementExportDownloadUrlResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signed(req.Header())
	f.urls = append(f.urls, proto.Clone(req.Msg).(*ledgerrdv1.GetStatementExportDownloadUrlRequest))
	return connect.NewResponse(f.url), nil
}

func (f *fakeStatementExports) ListStatementExports(_ context.Context, req *connect.Request[ledgerrdv1.ListStatementExportsRequest]) (*connect.Response[ledgerrdv1.ListStatementExportsResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signed(req.Header())
	return connect.NewResponse(f.list), nil
}

func newStatementExportsClient(t *testing.T, fake *fakeStatementExports, defaultSub *string) *Client {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := ledgerrdv1connect.NewStatementExportServiceHandler(fake)
	mux.Handle(path, handler)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	keypair := auth.GenerateEd25519Keypair()
	client, err := New(Config{APIURL: srv.URL, APIKeyID: "ak_statements", APIPrivateKey: keypair.SecretKeyHex, DefaultSubAccountID: defaultSub, SkipCatalogHydration: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestStatementsCreateExportScopes(t *testing.T) {
	fake := &fakeStatementExports{}
	defaultSub := codecs.FormatID(9000000001)
	client := newStatementExportsClient(t, fake, &defaultSub)
	start, end := time.Unix(1_790_000_000, 0), time.Unix(1_790_086_400, 123456789)
	explicit := codecs.FormatID(9000000002)
	empty := ""

	cases := []struct {
		name string
		opts services.CreateStatementExportOptions
		want *uint64
	}{
		{"default sub-account", services.CreateStatementExportOptions{}, proto.Uint64(9000000001)},
		{"explicit sub-account", services.CreateStatementExportOptions{SubAccountID: &explicit}, proto.Uint64(9000000002)},
		{"account main is the main account", services.CreateStatementExportOptions{Account: "main"}, nil},
		{"account active uses the default", services.CreateStatementExportOptions{Account: "active"}, proto.Uint64(9000000001)},
	}
	for _, tc := range cases {
		opts := tc.opts
		opts.StartTime, opts.EndTime, opts.AssetID, opts.IdempotencyKey = start, end, 3, "statement-1"
		export, err := client.Statements.CreateExport(context.Background(), opts)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if export.Status != "queued" || export.IsFinal() {
			t.Fatalf("%s: export = %+v", tc.name, export)
		}
		fake.mu.Lock()
		got := fake.creates[len(fake.creates)-1]
		fake.mu.Unlock()
		if (got.SubaccountId == nil) != (tc.want == nil) || (got.SubaccountId != nil && *got.SubaccountId != *tc.want) {
			t.Fatalf("%s: subaccount_id = %v, want %v", tc.name, got.SubaccountId, tc.want)
		}
		if !got.GetStartTime().AsTime().Equal(start) || !got.GetEndTime().AsTime().Equal(end) || got.GetAssetId() != 3 {
			t.Fatalf("%s: request = %v; want the period at full precision and the asset", tc.name, got)
		}
	}
	// An empty sub-account is no ID: it is refused before sending, never the
	// authenticated account.
	fake.mu.Lock()
	sent := len(fake.creates)
	fake.mu.Unlock()
	opts := services.CreateStatementExportOptions{SubAccountID: &empty, StartTime: start, EndTime: end, IdempotencyKey: "statement-1"}
	var validation *sdkerrors.ValidationError
	if _, err := client.Statements.CreateExport(context.Background(), opts); !stderrors.As(err, &validation) {
		t.Fatalf("empty sub-account: err = %v, want a ValidationError", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.creates) != sent || fake.unsigned != 0 {
		t.Fatalf("creates=%d (want %d) unsigned=%d", len(fake.creates), sent, fake.unsigned)
	}
}

func TestStatementsCancelExportReturnsTheCanceledExport(t *testing.T) {
	fake := &fakeStatementExports{cancel: &ledgerrdv1.CancelStatementExportResponse{Export: &ledgerrdv1.StatementExport{
		ExportId: 7000000001,
		Status:   ledgerrdv1.StatementExportStatus_STATEMENT_EXPORT_STATUS_CANCELED,
	}}}
	client := newStatementExportsClient(t, fake, nil)
	exportID := codecs.FormatID(7000000001)
	export, err := client.Statements.CancelExport(context.Background(), exportID)
	if err != nil {
		t.Fatal(err)
	}
	if export.ExportID != exportID || export.Status != "canceled" || !export.IsFinal() {
		t.Fatalf("export = %+v", export)
	}
	if len(fake.cancels) != 1 || fake.cancels[0].GetExportId() != 7000000001 || fake.unsigned != 0 {
		t.Fatalf("cancels = %v unsigned=%d", fake.cancels, fake.unsigned)
	}

	// A reply for another export breaks the contract.
	fake.cancel.Export.ExportId = 7000000002
	var contract *sdkerrors.ResponseContractError
	if _, err := client.Statements.CancelExport(context.Background(), exportID); !stderrors.As(err, &contract) {
		t.Fatalf("other export: err = %v, want *errors.ResponseContractError", err)
	}

	// A blank or malformed ID is refused before sending.
	for _, id := range []string{"", "0", "not-base58!"} {
		var validation *sdkerrors.ValidationError
		if _, err := client.Statements.CancelExport(context.Background(), id); !stderrors.As(err, &validation) {
			t.Fatalf("CancelExport(%q) err = %v, want a ValidationError", id, err)
		}
	}
	if len(fake.cancels) != 2 {
		t.Fatalf("sent %d cancels, want 2", len(fake.cancels))
	}
}

func TestStatementsGetExportDownloadURLNeverPrintsTheURL(t *testing.T) {
	const signed = "https://files.example/statement-7000000001.csv?signature=very-secret"
	expires := time.Date(2026, 11, 16, 21, 0, 0, 0, time.UTC)
	fake := &fakeStatementExports{url: &ledgerrdv1.GetStatementExportDownloadUrlResponse{
		Url:       signed,
		ExpiresAt: timestamppb.New(expires),
		Filename:  "statement-7000000001.csv",
	}}
	client := newStatementExportsClient(t, fake, nil)
	got, err := client.Statements.GetExportDownloadURL(context.Background(), codecs.FormatID(7000000001))
	if err != nil {
		t.Fatal(err)
	}
	if got.URL.Reveal() != signed || !got.ExpiresAt.Equal(expires) || got.Filename != "statement-7000000001.csv" {
		t.Fatalf("download URL = %+v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{fmt.Sprintf("%v %+v %#v %s", got, got, got, got.URL), string(encoded)} {
		if strings.Contains(text, "very-secret") {
			t.Fatalf("printed the URL: %s", text)
		}
	}
	if len(fake.urls) != 1 || fake.urls[0].GetExportId() != 7000000001 || fake.unsigned != 0 {
		t.Fatalf("requests = %v unsigned=%d", fake.urls, fake.unsigned)
	}

	// A reply without its URL or expiry breaks the contract.
	for _, reply := range []*ledgerrdv1.GetStatementExportDownloadUrlResponse{
		{ExpiresAt: timestamppb.New(expires)},
		{Url: signed},
	} {
		fake.url = reply
		_, err := client.Statements.GetExportDownloadURL(context.Background(), codecs.FormatID(7000000001))
		var contract *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contract) || strings.Contains(err.Error(), "very-secret") {
			t.Fatalf("reply %v: err = %v, want a *errors.ResponseContractError without the URL", reply, err)
		}
	}
}

func TestStatementsListExportsCarriesTheInProgressCounts(t *testing.T) {
	fake := &fakeStatementExports{list: &ledgerrdv1.ListStatementExportsResponse{
		Exports:              []*ledgerrdv1.StatementExport{{ExportId: 7000000001, Status: ledgerrdv1.StatementExportStatus_STATEMENT_EXPORT_STATUS_PROCESSING}},
		InProgressExports:    5,
		MaxInProgressExports: 5,
	}}
	client := newStatementExportsClient(t, fake, nil)
	list, err := client.Statements.ListExports(context.Background(), services.ListStatementExportsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := models.StatementExportsList{
		Exports:              []models.StatementExport{{ExportID: codecs.FormatID(7000000001), Status: "processing"}},
		InProgressExports:    5,
		MaxInProgressExports: 5,
	}
	if fmt.Sprintf("%+v", list) != fmt.Sprintf("%+v", want) {
		t.Fatalf("list = %+v, want %+v", list, want)
	}
}

func TestStatementExportIsFinal(t *testing.T) {
	for status, final := range map[string]bool{
		"queued": false, "processing": false,
		"ready": true, "failed": true, "expired": true, "canceled": true, "unknown(9)": true,
	} {
		if got := (models.StatementExport{Status: status}).IsFinal(); got != final {
			t.Fatalf("IsFinal(%q) = %v, want %v", status, got, final)
		}
	}
}
