package services_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/services"
)

// "main" always addresses the main account, whatever default sub-account
// the client has. Only an unset account (nil) and "active" take the default.
// An empty account is no selector and no ID: it is refused.
func TestScopedSubAccountIDMainIgnoresDefault(t *testing.T) {
	defaultSub := "Bht3tfQZv8E"
	for _, tc := range []struct {
		name    string
		account services.AccountScope
		want    *string
	}{
		{name: "unset takes the default", account: nil, want: &defaultSub},
		{name: "active takes the default", account: "active", want: &defaultSub},
		{name: "main is the main account", account: "main", want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := services.ScopedSubAccountID(tc.account, nil, &defaultSub)
			if err != nil {
				t.Fatal(err)
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("ScopedSubAccountID(%#v) = %v, want %v", tc.account, deref(got), deref(tc.want))
			}
		})
	}
}

// With no default configured, every selector but an explicit sub-account
// addresses the main account, as before. An empty account is refused.
func TestScopedSubAccountIDWithoutDefault(t *testing.T) {
	for _, account := range []services.AccountScope{nil, "active", "main"} {
		got, err := services.ScopedSubAccountID(account, nil, nil)
		if err != nil || got != nil {
			t.Fatalf("ScopedSubAccountID(%#v) = %v, %v; want main account", account, deref(got), err)
		}
	}
	defaultSub := "Bht3tfQZv8E"
	for _, def := range []*string{nil, &defaultSub} {
		if got, err := services.ScopedSubAccountID("", nil, def); err == nil {
			t.Fatalf("ScopedSubAccountID(\"\") = %v, want a ValidationError", deref(got))
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<main>"
	}
	return *s
}
