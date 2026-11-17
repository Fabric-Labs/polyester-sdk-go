package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// blankIDs are supplied IDs that hold no base58 public ID. Each is refused
// wherever a call takes an ID; none means "unset".
var blankIDs = []string{"", " ", "\t", " ", " 23", "23 "}

func requireValidation(t *testing.T, name string, err error) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) {
		t.Errorf("%s: err = %T %v, want a ValidationError", name, err, err)
	}
}

// A sub-account scope reads a supplied sub-account, an account selector or a
// configured default sub-account strictly: a blank one is refused, never the
// main account. Only nil, or the account selector "main", is the main account.
func TestSubAccountScopeRefusesABlankSubAccount(t *testing.T) {
	for _, id := range blankIDs {
		id := id
		_, err := (&ScopedSubAccount{}).ResolveSubAccountID(&id, nil)
		requireValidation(t, "sub_account_id "+id, err)
		_, err = (&ScopedSubAccount{}).ResolveSubAccountID(nil, id)
		requireValidation(t, "account "+id, err)
		_, err = (&ScopedSubAccount{}).ResolveSubAccountID(nil, map[string]string{"subaccountId": id})
		requireValidation(t, "account map "+id, err)
		_, err = (&ScopedSubAccount{DefaultSubAccountID: &id}).ResolveSubAccountID(nil, nil)
		requireValidation(t, "default sub_account_id "+id, err)
		_, err = codecs.ParseOptionalSubaccountID(&id)
		requireValidation(t, "ParseOptionalSubaccountID "+id, err)
	}
	for name, account := range map[string]AccountScope{"nil": nil, "main": "main"} {
		got, err := (&ScopedSubAccount{}).ResolveSubAccountID(nil, account)
		if err != nil || got != nil {
			t.Errorf("%s: sub-account = %v, %v; want the main account", name, got, err)
		}
	}
	def := codecs.FormatID(5)
	got, err := (&ScopedSubAccount{DefaultSubAccountID: &def}).ResolveSubAccountID(nil, nil)
	if err != nil || got == nil || *got != def {
		t.Errorf("unset sub-account with a default = %v, %v; want %q", got, err, def)
	}
	if field, err := codecs.ParseOptionalSubaccountID(nil); err != nil || field != nil {
		t.Errorf("ParseOptionalSubaccountID(nil) = %v, %v", field, err)
	}
}

// A blank internal-transfer destination ID is refused; it does not drop out
// and leave the transfer to another destination.
func TestInternalTransferRefusesABlankDestinationID(t *testing.T) {
	address := "0x2222222222222222222222222222222222222222"
	for _, id := range blankIDs {
		id := id
		_, err := internalTransferDestination(&id, nil, &address)
		requireValidation(t, "destination_account_id "+id+" with an address", err)
		_, err = internalTransferDestination(nil, &id, &address)
		requireValidation(t, "destination_subaccount_id "+id+" with an address", err)
		_, err = internalTransferDestination(&id, nil, nil)
		requireValidation(t, "destination_account_id "+id, err)
	}
	if dest, err := internalTransferDestination(nil, nil, &address); err != nil || dest.address != address {
		t.Errorf("address only: %+v, %v", dest, err)
	}
}

// SubscribeOpenFlows takes the public feed only when no account is given; a
// blank account ID is refused, never read as "public".
func TestSubscribeOpenFlowsRefusesABlankAccountInsteadOfThePublicFeed(t *testing.T) {
	fake := newFakeCentrifugo(t)
	rt := realtime.NewClient(fake.wsURL, fake.apiURL, accountIDTestCredentials(t), nil, 16)
	t.Cleanup(rt.Close)
	lifecycle := NewLifecycleService(nil, rt, nil)
	for _, id := range blankIDs {
		sub, err := lifecycle.SubscribeOpenFlows(context.Background(), id)
		if sub != nil {
			sub.Close()
		}
		requireValidation(t, "SubscribeOpenFlows "+id, err)
		if channels, _ := fake.take(); len(channels) != 0 {
			t.Errorf("SubscribeOpenFlows(%q) subscribed %v", id, channels)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub, err := lifecycle.SubscribeOpenFlows(ctx, nil)
	if err != nil {
		t.Fatalf("SubscribeOpenFlows(nil): %v", err)
	}
	sub.Close()
	// The public feed needs no private-channel token.
	if channels, _ := fake.take(); len(channels) != 0 {
		t.Fatalf("SubscribeOpenFlows(nil) requested private channels %v, want the public feed", channels)
	}
}

// A supplied flow ID is read by the contract's flow_id rule: a blank or
// spaced one is refused before sending, and GetFlow never falls back from a
// supplied flow ID to the intent ID. A nil flow ID still reads the intent.
func TestFlowLookupsRefuseABlankFlowID(t *testing.T) {
	factory, captures := scopeCaptureFactory(t)
	fake := newFakeCentrifugo(t)
	rt := realtime.NewClient(fake.wsURL, fake.apiURL, accountIDTestCredentials(t), nil, 16)
	t.Cleanup(rt.Close)
	lifecycle := NewLifecycleService(factory, rt, nil)
	ctx := context.Background()
	intent := "flow_23"
	for _, id := range append(blankIDs, " flow_23", "flow_23 ") {
		id := id
		_, err := lifecycle.GetFlow(ctx, &intent, &id)
		requireValidation(t, "GetFlow "+id, err)
		_, err = lifecycle.GetAccountFlow(ctx, id, nil, nil)
		requireValidation(t, "GetAccountFlow "+id, err)
		sub, err := lifecycle.SubscribeFlowDetail(ctx, id)
		if sub != nil {
			sub.Close()
		}
		requireValidation(t, "SubscribeFlowDetail "+id, err)
		if len(captures) != 0 {
			<-captures
			t.Errorf("flow ID %q reached transport", id)
		}
	}
	_, _ = lifecycle.GetFlow(ctx, &intent, nil)
	if len(captures) != 1 {
		t.Fatalf("GetFlow(intent, nil) captured %d requests, want one", len(captures))
	}
}
