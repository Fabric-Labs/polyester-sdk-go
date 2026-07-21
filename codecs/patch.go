package codecs

import (
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ClearableTime distinguishes omit vs clear vs set for timestamp patch fields.
// Use a nil *ClearableTime to omit the field. Use &ClearableTime{} (Time == nil)
// to clear. Use &ClearableTime{Time: &t} to set.
type ClearableTime struct {
	Time *time.Time
}

// TimeClear returns a ClearableTime that clears the field when selected.
func TimeClear() *ClearableTime { return &ClearableTime{} }

// TimeSet returns a ClearableTime that sets the field when selected.
func TimeSet(t time.Time) *ClearableTime { return &ClearableTime{Time: &t} }

// Ptr returns a pointer to v for optional patch fields.
func Ptr[T any](v T) *T { return &v }

func requirePositiveRevision(rev uint64) error {
	if rev == 0 {
		return &errors.ValidationError{Msg: "expected_revision must be positive"}
	}
	return nil
}

func newUpdateMask(paths []string) (*fieldmaskpb.FieldMask, error) {
	if len(paths) == 0 {
		return nil, &errors.ValidationError{Msg: "update_mask must be non-empty"}
	}
	return &fieldmaskpb.FieldMask{Paths: paths}, nil
}

func timestampFromClearable(c *ClearableTime, field string) (*timestamppb.Timestamp, error) {
	if c == nil || c.Time == nil {
		return nil, nil
	}
	if c.Time.Location() == time.Local {
		return nil, &errors.ValidationError{Msg: field + " must be timezone-aware"}
	}
	return timestamppb.New(c.Time.UTC()), nil
}
