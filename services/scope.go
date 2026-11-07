package services

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// AccountScope mirrors TS-style account routing: "main", "active", subaccount id, or scoped dict.
type AccountScope any

// ScopedSubAccount holds default sub-account resolution for trading services.
type ScopedSubAccount struct {
	DefaultSubAccountID *string
}

// ResolveSubAccountID resolves empty string to nil.
func ResolveSubAccountID(value *string, defaultVal *string) *string {
	if value != nil && *value == "" {
		return nil
	}
	if value != nil {
		return value
	}
	if defaultVal != nil && *defaultVal == "" {
		return nil
	}
	return defaultVal
}

// ScopedSubAccountID resolves sub-account scope from account or legacy sub_account_id.
func ScopedSubAccountID(account AccountScope, subAccountID *string, defaultVal *string) (*string, error) {
	if subAccountID != nil && account != nil {
		return nil, &errors.ValidationError{Msg: "Pass account or sub_account_id, not both"}
	}
	if subAccountID != nil {
		return ResolveSubAccountID(subAccountID, defaultVal), nil
	}
	if account == nil || account == "active" || account == "main" {
		return ResolveSubAccountID(nil, defaultVal), nil
	}
	switch v := account.(type) {
	case map[string]string:
		scoped := v["subaccountId"]
		if scoped == "" {
			scoped = v["sub_account_id"]
		}
		if scoped == "" {
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
// defaultVal when value is unset (nil, a nil pointer or ""). Each may be a
// public ID string, which is used as given, or a positive integer, which is
// encoded as its public ID (42 is "j"). Any other value is a ValidationError.
func ResolveAccountID(value any, defaultVal any) (string, error) {
	for _, v := range []any{value, defaultVal} {
		id, _, err := publicID(v, "account_id")
		if err != nil {
			return "", err
		}
		if id != "" {
			return id, nil
		}
	}
	return "", &errors.ValidationError{Msg: "account_id is required; set POLYESTER_ACCOUNT_ID or pass account_id explicitly"}
}

// publicID returns the public ID text of an account or sub-account ID given
// as a public ID string or a positive integer; set is false when value is nil
// or a nil pointer. A string or *string is returned unchanged, and a named
// string type trimmed. An integer is encoded with codecs.FormatID, never as
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
		return strings.TrimSpace(v.String()), true, nil
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

// LifecycleAccountFields extracts owner account fields from a proto-like message.
func LifecycleAccountFields(ownerAccountID uint64, smartAccountAddress string) map[string]string {
	fields := map[string]string{}
	if ownerAccountID != 0 {
		fields["owner_account_id"] = codecs.FormatID(ownerAccountID)
	}
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
