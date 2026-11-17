package services

import (
	"fmt"
	"math/big"
	"reflect"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// AccountScope selects the account a call acts on:
//
//   - nil (unset) or "active": the client's DefaultSubAccountID, or the main
//     account when none is configured;
//   - "main": always the main account, whatever DefaultSubAccountID is;
//   - any other string: that sub-account's base58 public ID, read strictly:
//     "", a blank string or any other text that is no public ID is refused;
//   - map[string]string{"subaccountId": id} (or "sub_account_id"): that
//     sub-account.
//
// Any other value is a *errors.ValidationError. Pass either an account or a
// sub-account ID, not both.
type AccountScope any

// ScopedSubAccount holds default sub-account resolution for trading services.
type ScopedSubAccount struct {
	DefaultSubAccountID *string
}

// ResolveSubAccountID returns value, or defaultVal when value is nil. Only
// nil is unset: a supplied ID, empty or blank included, is returned as given
// for the strict public-ID parser to read.
func ResolveSubAccountID(value *string, defaultVal *string) *string {
	if value != nil {
		return value
	}
	return defaultVal
}

// ScopedSubAccountID resolves the sub-account of a call from account (see
// AccountScope) or the legacy subAccountID, falling back to defaultVal only
// when both are unset or account is "active". nil means the main account.
func ScopedSubAccountID(account AccountScope, subAccountID *string, defaultVal *string) (*string, error) {
	sub, err := scopedSubAccountID(account, subAccountID, defaultVal)
	if err != nil || sub == nil {
		return sub, err
	}
	// A supplied, selected or default sub-account is a public ID: a blank or
	// malformed one is refused here, never read as the main account.
	if _, err := codecs.ParseAccountID(*sub, "sub_account_id"); err != nil {
		return nil, err
	}
	return sub, nil
}

func scopedSubAccountID(account AccountScope, subAccountID *string, defaultVal *string) (*string, error) {
	if subAccountID != nil && account != nil {
		return nil, &errors.ValidationError{Msg: "Pass account or sub_account_id, not both"}
	}
	if subAccountID != nil {
		return ResolveSubAccountID(subAccountID, defaultVal), nil
	}
	if account == nil || account == "active" {
		return ResolveSubAccountID(nil, defaultVal), nil
	}
	if account == "main" {
		return nil, nil
	}
	switch v := account.(type) {
	case map[string]string:
		scoped, ok := v["subaccountId"]
		if !ok {
			scoped, ok = v["sub_account_id"]
		}
		if !ok {
			return nil, &errors.ValidationError{Msg: "account dict requires subaccountId or sub_account_id"}
		}
		s := scoped
		return &s, nil
	case string:
		s := v
		return ResolveSubAccountID(&s, defaultVal), nil
	default:
		return nil, &errors.ValidationError{Msg: "account must be 'main', 'active', a subaccount id, or a dict"}
	}
}

// ResolveAccountID returns the public account ID from value, or from
// defaultVal when value is unset (nil or a nil pointer). Each may be a public
// ID string, which must be base58 as the API reads it, or a positive integer,
// which is encoded as its public ID (42 is "j"). A supplied string is never
// unset: an empty or blank one, one that decodes to 0 such as "1", and any
// other value are a ValidationError.
func ResolveAccountID(value any, defaultVal any) (string, error) {
	for _, v := range []any{value, defaultVal} {
		id, set, err := publicID(v, "account_id")
		if err != nil {
			return "", err
		}
		if set {
			if _, err := codecs.ParseAccountID(id, "account_id"); err != nil {
				return "", err
			}
			return id, nil
		}
	}
	return "", &errors.ValidationError{Msg: "account_id is required; set Config.DefaultAccountID or pass account_id explicitly"}
}

// publicID returns the public ID text of an account or sub-account ID given
// as a public ID string or a positive integer; set is false when value is nil
// or a nil pointer. A string, *string or named string type is returned
// unchanged, never trimmed. An integer is encoded with codecs.FormatID, never as
// decimal text, which reads as the public ID of another account. Zero,
// negative, out-of-range, bool and other values are refused.
func publicID(value any, label string) (id string, set bool, err error) {
	invalid := &errors.ValidationError{Msg: label + " must be a positive uint64 public ID"}
	switch t := value.(type) {
	case string:
		return t, true, nil
	case *string:
		if t == nil {
			return "", false, nil
		}
		return *t, true, nil
	case *big.Int:
		if t == nil {
			return "", false, nil
		}
		if t.Sign() <= 0 || !t.IsUint64() {
			return "", false, invalid
		}
		return codecs.FormatID(t.Uint64()), true, nil
	}
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "", false, nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Invalid:
		return "", false, nil
	case reflect.String:
		return v.String(), true, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Int() <= 0 {
			return "", false, invalid
		}
		return codecs.FormatID(uint64(v.Int())), true, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Uint() == 0 {
			return "", false, invalid
		}
		return codecs.FormatID(v.Uint()), true, nil
	default:
		return "", false, &errors.ValidationError{Msg: fmt.Sprintf("%s must be a public ID string or a positive integer, not %T", label, value)}
	}
}

// LifecycleAccountFields names the account of a lifecycle flow. A flow names
// its account only by its smart account address, the account's public
// identity: the contract no longer carries an owner account ID.
func LifecycleAccountFields(smartAccountAddress string) map[string]string {
	fields := map[string]string{}
	if smartAccountAddress != "" {
		fields["smart_account_address"] = smartAccountAddress
	}
	return fields
}

// ResolveSubAccountID resolves the sub-account from subAccountID, a public ID
// string or a positive integer (encoded as its public ID), or from account.
func (s *ScopedSubAccount) ResolveSubAccountID(subAccountID any, account AccountScope) (*string, error) {
	id, set, err := publicID(subAccountID, "sub_account_id")
	if err != nil {
		return nil, err
	}
	var explicit *string
	if set {
		explicit = &id
	}
	return ScopedSubAccountID(account, explicit, s.DefaultSubAccountID)
}

// ApplyOptionalSubaccountID sets a plain uint64 subaccount field when scope resolves one.
func (s *ScopedSubAccount) ApplyOptionalSubaccountID(dst *uint64, account AccountScope, subAccountID *string) error {
	sub, err := s.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return err
	}
	parsed, err := codecs.ParseOptionalSubaccountID(sub)
	if err != nil {
		return err
	}
	if parsed != nil {
		*dst = *parsed
	}
	return nil
}

// ApplyOptionalSubaccountIDPtr sets an optional *uint64 subaccount field when scope resolves one.
func (s *ScopedSubAccount) ApplyOptionalSubaccountIDPtr(dst **uint64, account AccountScope, subAccountID *string) error {
	sub, err := s.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return err
	}
	parsed, err := codecs.ParseOptionalSubaccountID(sub)
	if err != nil {
		return err
	}
	if parsed != nil {
		*dst = parsed
	}
	return nil
}
