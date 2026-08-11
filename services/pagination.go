package services

import (
	"fmt"
	"math"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// PaginationLimit converts an optional int limit without wrapping negatives or
// values above the protobuf uint32 range. A zero value preserves omission.
func PaginationLimit(limit int, label string) (uint32, error) {
	if limit < 0 {
		return 0, &errors.ValidationError{Msg: label + " must be non-negative"}
	}
	if uint64(limit) > math.MaxUint32 {
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("%s %d exceeds uint32 range", label, limit)}
	}
	return uint32(limit), nil
}

// PaginationLimitOrDefault applies an existing default only when limit is
// omitted as zero. Negative and overflowing explicit values are rejected.
func PaginationLimitOrDefault(limit int, fallback uint32, label string) (uint32, error) {
	if limit == 0 {
		return fallback, nil
	}
	return PaginationLimit(limit, label)
}

// ExplicitPaginationLimit validates a pointer-shaped explicitly supplied
// limit. nil preserves omission; explicit zero is invalid.
func ExplicitPaginationLimit(limit *int, label string) (*uint32, error) {
	if limit == nil {
		return nil, nil
	}
	if *limit <= 0 {
		return nil, &errors.ValidationError{Msg: label + " must be positive when explicitly supplied"}
	}
	value, err := PaginationLimit(*limit, label)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
