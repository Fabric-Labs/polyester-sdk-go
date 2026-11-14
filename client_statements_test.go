package polyester

import (
	"context"
	stderrors "errors"
	"net/http"
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
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"google.golang.org/protobuf/proto"
)

type fakeStatements struct {
	ledgerrdv1connect.UnimplementedLedgerReadServiceHandler

	mu       sync.Mutex
	requests []*ledgerrdv1.ExportStatementRequest
	unsigned int
	pages    map[string]*ledgerrdv1.ExportStatementResponse
}

func (f *fakeStatements) ExportStatement(_ context.Context, req *connect.Request[ledgerrdv1.ExportStatementRequest]) (*connect.Response[ledgerrdv1.ExportStatementResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, proto.Clone(req.Msg).(*ledgerrdv1.ExportStatementRequest))
	if req.Header().Get("X-API-KEY-ID") == "" || req.Header().Get("X-API-SIGNATURE") == "" {
		f.unsigned++
	}
	page, ok := f.pages[req.Msg.GetPageToken()]
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, stderrors.New("unknown page token"))
	}
	return connect.NewResponse(page), nil
}

func newStatementsClient(t *testing.T, fake *fakeStatements, defaultSub *string) *Client {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := ledgerrdv1connect.NewLedgerReadServiceHandler(fake)
	mux.Handle(path, handler)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	keypair := auth.GenerateEd25519Keypair()
	client, err := New(Config{APIURL: srv.URL, APIKeyID: "ak_statements", APIPrivateKey: keypair.SecretKeyHex, DefaultSubAccountID: defaultSub})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

const statementHeader = "time,entry_id,type,account,asset,amount,fee,balance,link_id,flow_id\r\n"

func TestStatementsExportFollowsPagesWithTheSameFieldsAndTheDefaultSubAccount(t *testing.T) {
	fake := &fakeStatements{pages: map[string]*ledgerrdv1.ExportStatementResponse{
		"": {
			Csv:           statementHeader + "2026-10-02T14:03:11.000000Z,27419385013475392777,deposit,funding,USDT,100,0,100,,flow_8d1f2c\r\n",
			NextPageToken: "p2",
		},
		"p2": {Csv: "2026-10-03T08:12:45.123456Z,27419385013475392811,trade,trading,USDT,-5000,2.5,4997.5,77,\r\n"},
	}}
	defaultSub := codecs.FormatID(9000000001)
	client := newStatementsClient(t, fake, &defaultSub)

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 1, 0, 0, 0, 123456789, time.UTC)
	statement, err := client.Statements.Export(context.Background(), services.ExportStatementOptions{StartTime: start, EndTime: end, AssetID: 3})
	if err != nil {
		t.Fatal(err)
	}
	wantCSV := fake.pages[""].GetCsv() + fake.pages["p2"].GetCsv()
	if statement.CSV != wantCSV {
		t.Fatalf("CSV = %q, want %q", statement.CSV, wantCSV)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.requests) != 2 || fake.unsigned != 0 {
		t.Fatalf("requests=%d unsigned=%d; want two signed requests", len(fake.requests), fake.unsigned)
	}
	for i, req := range fake.requests {
		if req.SubaccountId == nil || *req.SubaccountId != 9000000001 {
			t.Fatalf("request %d subaccount_id = %v, want the default sub-account", i, req.SubaccountId)
		}
		if !req.GetStartTime().AsTime().Equal(start) || !req.GetEndTime().AsTime().Equal(end) || req.GetAssetId() != 3 {
			t.Fatalf("request %d = %v; want the same period (full precision) and asset on every page", i, req)
		}
	}
	if fake.requests[0].GetPageToken() != "" || fake.requests[1].GetPageToken() != "p2" {
		t.Fatalf("page tokens = %q, %q", fake.requests[0].GetPageToken(), fake.requests[1].GetPageToken())
	}

	rows, err := statement.Rows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].FlowID != "flow_8d1f2c" || rows[1].Fee != "2.5" || rows[1].LinkID != "77" ||
		!rows[1].Time.Equal(time.Date(2026, 10, 3, 8, 12, 45, 123456000, time.UTC)) {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestStatementsExportPageScopes(t *testing.T) {
	fake := &fakeStatements{pages: map[string]*ledgerrdv1.ExportStatementResponse{"": {Csv: statementHeader}}}
	defaultSub := codecs.FormatID(9000000001)
	client := newStatementsClient(t, fake, &defaultSub)
	start, end := time.Unix(1_790_000_000, 0), time.Unix(1_790_086_400, 0)
	explicit := codecs.FormatID(9000000002)
	empty := ""

	cases := []struct {
		name string
		opts services.ExportStatementOptions
		want *uint64
	}{
		{"explicit sub-account", services.ExportStatementOptions{SubAccountID: &explicit}, proto.Uint64(9000000002)},
		{"account main uses the default", services.ExportStatementOptions{Account: "main"}, proto.Uint64(9000000001)},
		{"empty sub-account is the authenticated account", services.ExportStatementOptions{SubAccountID: &empty}, nil},
	}
	for _, tc := range cases {
		opts := tc.opts
		opts.StartTime, opts.EndTime = start, end
		page, err := client.Statements.ExportPage(context.Background(), opts)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if page.CSV != statementHeader || page.NextPageToken != "" {
			t.Fatalf("%s: page = %+v", tc.name, page)
		}
		fake.mu.Lock()
		got := fake.requests[len(fake.requests)-1].SubaccountId
		fake.mu.Unlock()
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Fatalf("%s: subaccount_id = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestStatementsExportRequiresThePeriodBeforeSending(t *testing.T) {
	fake := &fakeStatements{}
	client := newStatementsClient(t, fake, nil)
	for _, opts := range []services.ExportStatementOptions{
		{EndTime: time.Now()},
		{StartTime: time.Now()},
	} {
		_, err := client.Statements.ExportPage(context.Background(), opts)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Fatalf("ExportPage(%+v) error = %v, want *errors.ValidationError", opts, err)
		}
	}
	if len(fake.requests) != 0 {
		t.Fatalf("sent %d requests, want none", len(fake.requests))
	}
}

func TestStatementsExportRefusesARepeatedPageToken(t *testing.T) {
	fake := &fakeStatements{pages: map[string]*ledgerrdv1.ExportStatementResponse{
		"":   {Csv: statementHeader, NextPageToken: "p2"},
		"p2": {Csv: "", NextPageToken: "p2"},
	}}
	client := newStatementsClient(t, fake, nil)
	_, err := client.Statements.Export(context.Background(), services.ExportStatementOptions{StartTime: time.Unix(1, 0), EndTime: time.Unix(2, 0)})
	var contract *sdkerrors.ResponseContractError
	if !stderrors.As(err, &contract) {
		t.Fatalf("Export error = %v, want *errors.ResponseContractError", err)
	}
}
