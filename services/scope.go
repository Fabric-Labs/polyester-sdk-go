package services

import (
	"fmt"
	"strconv"
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

// ResolveAccountID returns account_id from value or default.
func ResolveAccountID(value any, defaultVal any) (string, error) {
	if s := stringish(value); s != "" {
		return s, nil
	}
	if s := stringish(defaultVal); s != "" {
		return s, nil
	}
	return "", &errors.ValidationError{Msg: "account_id is required; set POLYESTER_ACCOUNT_ID or pass account_id explicitly"}
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

func (s *ScopedSubAccount) ResolveSubAccountID(subAccountID any, account AccountScope) (*string, error) {
	return ScopedSubAccountID(account, optionalStringPtr(subAccountID), s.DefaultSubAccountID)
}

func optionalStringPtr(v any) *string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case *string:
		if t == nil || *t == "" {
			return nil
		}
		return t
	case string:
		if t == "" {
			return nil
		}
		s := t
		return &s
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" {
			return nil
		}
		return &s
	}
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

func stringish(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	default:
		s := fmt.Sprint(v)
		if s == "<nil>" {
			return ""
		}
		return s
	}
}
