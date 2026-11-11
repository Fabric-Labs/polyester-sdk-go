package wire

import (
	"errors"
	"reflect"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
)

// redactChain passes every string field of every error in err's chain, and
// of the structs and slices of structs those errors hold, through
// redact.Text, in place. The errors are the ones MapConnectError has just
// built or taken from the transport for this call, so no other caller holds
// them.
func redactChain(err error) {
	seen := map[error]bool{}
	pending := []error{err}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == nil || !reflect.TypeOf(current).Comparable() || seen[current] {
			continue
		}
		seen[current] = true
		value := reflect.ValueOf(current)
		if value.Kind() == reflect.Pointer && !value.IsNil() {
			redactValue(value.Elem())
		}
		switch wrapped := current.(type) {
		case interface{ Unwrap() error }:
			pending = append(pending, wrapped.Unwrap())
		case interface{ Unwrap() []error }:
			pending = append(pending, wrapped.Unwrap()...)
		default:
			pending = append(pending, errors.Unwrap(current))
		}
	}
}

func redactValue(value reflect.Value) {
	switch value.Kind() {
	case reflect.String:
		if value.CanSet() {
			value.SetString(redact.Text(value.String()))
		}
	case reflect.Struct:
		for i := range value.NumField() {
			redactValue(value.Field(i))
		}
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Struct || value.Type().Elem().Kind() == reflect.String {
			for i := range value.Len() {
				redactValue(value.Index(i))
			}
		}
	}
}
