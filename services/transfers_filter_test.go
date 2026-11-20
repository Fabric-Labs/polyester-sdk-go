package services

import (
	"context"
	stderrors "errors"
	"math"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ledger "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	ledgerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/v1"
)

// ListWith sends the transfer reason filter with the other filters, and
// continues a filtered listing with the same reason; List sends none.
func TestTransfersListWithSendsTheTransferCode(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			h := &readPreservationHandler{}
			factory, _ := newReadPreservationServices(t, h, wire)
			service := NewTransfersService(factory, nil, nil, nil)
			since := int64(1_700_000_000_000_000)
			params := ListTransfersParams{Limit: 10, Reversed: true, Since: &since, TransferCode: int(ledgerv1.TransferCode_DEPOSIT)}
			first, err := service.ListWith(context.Background(), params)
			if err != nil {
				t.Fatal(err)
			}
			params.PageToken = first.NextPageToken
			if _, err := service.ListWith(context.Background(), params); err != nil {
				t.Fatal(err)
			}
			if _, err := service.List(context.Background(), nil, nil, 10, false, nil); err != nil {
				t.Fatal(err)
			}
			if len(h.calls) != 3 {
				t.Fatalf("calls = %d, want 3", len(h.calls))
			}
			for i, want := range []string{"", first.NextPageToken} {
				req := h.calls[i].(*ledger.ListTransfersRequest)
				if req.GetTransferCode() != ledgerv1.TransferCode_DEPOSIT || req.GetPageToken() != want ||
					req.GetTsMinUs() != uint64(since) || !req.GetReversed() || req.GetLimit() != 10 {
					t.Fatalf("call %d = %v", i, req)
				}
			}
			if code := h.calls[2].(*ledger.ListTransfersRequest).GetTransferCode(); code != ledgerv1.TransferCode_TRANSFER_CODE_UNSPECIFIED {
				t.Fatalf("List sent transfer_code %v", code)
			}
		})
	}
}

// A transfer code outside the enum's number range is refused before sending.
func TestTransfersListWithRefusesAnInvalidTransferCode(t *testing.T) {
	h := &readPreservationHandler{}
	factory, _ := newReadPreservationServices(t, h, "binary")
	service := NewTransfersService(factory, nil, nil, nil)
	tooLarge := int64(math.MaxInt32) + 1
	for _, code := range []int{-1, int(tooLarge)} {
		_, err := service.ListWith(context.Background(), ListTransfersParams{TransferCode: code})
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Fatalf("transfer code %d: err = %v, want a ValidationError", code, err)
		}
	}
	if len(h.calls) != 0 {
		t.Fatalf("calls = %d, want none", len(h.calls))
	}
}
