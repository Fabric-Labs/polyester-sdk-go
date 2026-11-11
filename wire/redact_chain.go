package wire

import (
	"errors"
	"reflect"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
)

// redactChain passes every string reachable from every error in err's chain
// through redact.Text, in place: fields, the structs and slices they hold,
// what their pointers and interfaces point to (such as a problem or a
// rate-limit detail), and map values. The errors are the ones
// MapConnectError has just built or taken from the transport for this call,
// so no other caller holds them.
func redactChain(err error) {
	seenErrors := map[error]bool{}
	seenPointers := map[uintptr]bool{}
	pending := []error{err}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == nil || !reflect.TypeOf(current).Comparable() || seenErrors[current] {
			continue
		}
		seenErrors[current] = true
		redactValue(reflect.ValueOf(current), seenPointers)
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

func redactValue(value reflect.Value, seen map[uintptr]bool) {
	switch value.Kind() {
	case reflect.String:
		if value.CanSet() {
			value.SetString(redact.Text(value.String()))
		}
	case reflect.Pointer:
		if value.IsNil() || seen[value.Pointer()] {
			return
		}
		seen[value.Pointer()] = true
		redactValue(value.Elem(), seen)
	case reflect.Interface:
		if !value.IsNil() {
			inner := value.Elem()
			if inner.Kind() == reflect.Pointer {
				redactValue(inner, seen)
			} else if value.CanSet() && inner.Kind() == reflect.String {
				value.Set(reflect.ValueOf(redact.Text(inner.String())).Convert(inner.Type()))
			}
		}
	case reflect.Struct:
		for i := range value.NumField() {
			redactValue(value.Field(i), seen)
		}
	case reflect.Slice, reflect.Array:
		for i := range value.Len() {
			redactValue(value.Index(i), seen)
		}
	case reflect.Map:
		if value.IsNil() {
			return
		}
		for _, key := range value.MapKeys() {
			copied := reflect.New(value.Type().Elem()).Elem()
			copied.Set(value.MapIndex(key))
			redactValue(copied, seen)
			value.SetMapIndex(key, copied)
		}
	}
}
