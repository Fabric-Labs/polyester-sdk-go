package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// A public ID that decodes to 0 ("1" is the base58 form of 0, "0" and "00"
// the decimal) names nothing: every call that takes one refuses it locally
// with a ValidationError and sends no request.
func TestCallsRefusePublicIDThatDecodesToZero(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	factory := transport.NewFactory(cfg, creds, nil)
	triggers := NewTriggersService(factory, nil, nil, nil, nil, nil, nil)
	orders := NewOrdersService(factory, nil, nil, nil, nil, nil, nil)
	addressBook := NewAddressBookService(factory, nil, nil, nil)
	symbolID := uint32(1)
	ctx := context.Background()

	calls := map[string]func(id string) error{
		"triggers.Get": func(id string) error { _, err := triggers.Get(ctx, nil, id, nil); return err },
		"triggers.Cancel": func(id string) error {
			_, err := triggers.Cancel(ctx, nil, id, nil)
			return err
		},
		"triggers.Pause": func(id string) error {
			_, err := triggers.Pause(ctx, nil, id, nil)
			return err
		},
		"triggers.Resume": func(id string) error {
			_, err := triggers.Resume(ctx, nil, id, nil, nil, &symbolID)
			return err
		},
		"triggers.ListEvents": func(id string) error {
			_, err := triggers.ListEvents(ctx, nil, id, nil, 0, nil, nil)
			return err
		},
		"triggers.CancelOco": func(id string) error {
			_, err := triggers.CancelOco(ctx, nil, id, nil)
			return err
		},
		"orders.Get": func(id string) error {
			_, err := orders.Get(ctx, nil, models.OrderKeyByID(id), nil, false, false)
			return err
		},
		"orders.Cancel": func(id string) error {
			_, err := orders.Cancel(ctx, nil, models.OrderKeyByID(id), nil, &symbolID, nil)
			return err
		},
		"orders.GetBatchReplaceStatus": func(id string) error {
			_, err := orders.GetBatchReplaceStatus(ctx, nil, id, nil)
			return err
		},
		"addressBook.DeleteEntry": func(id string) error { return addressBook.DeleteEntry(ctx, id) },
		"addressBook.DeleteTag":   func(id string) error { return addressBook.DeleteTag(ctx, id) },
	}
	for name, call := range calls {
		for _, id := range []string{"1", "0", "00"} {
			err := call(id)
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || !strings.Contains(validation.Msg, "non-zero") {
				t.Errorf("%s(%q): got %v, want a non-zero ValidationError", name, id, err)
			}
		}
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("%d requests sent for a zero ID", n)
	}
}

// The shared parser refuses a zero public ID and still decodes others.
func TestIDToIntRefusesZero(t *testing.T) {
	for _, raw := range []string{"1", "0", "00", " 1 "} {
		if got, err := codecs.IDToInt(raw, "trigger_id"); err == nil {
			t.Errorf("IDToInt(%q) = %d, want an error", raw, got)
		}
	}
	for raw, want := range map[string]uint64{"2": 1, "5": 4, "10": 10, codecs.FormatID(1 << 63): 1 << 63} {
		if got, err := codecs.IDToInt(raw, "trigger_id"); err != nil || got != want {
			t.Errorf("IDToInt(%q) = (%d, %v), want %d", raw, got, err, want)
		}
	}
}
