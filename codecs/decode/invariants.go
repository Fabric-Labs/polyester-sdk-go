package decode

import (
	"fmt"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

const (
	minMillisecondEpoch = uint64(100_000_000_000)
	minNanosecondEpoch  = uint64(1_000_000_000_000_000)
)

// ValidateTimestampNS rejects epoch-millisecond-shaped values in fields whose
// response contract explicitly declares nanoseconds. Zero and small synthetic
// sequence-style fixture values remain valid.
func ValidateTimestampNS(value uint64, operation, field string) error {
	if value >= minMillisecondEpoch && value < minNanosecondEpoch {
		return &sdkerrors.ResponseContractError{
			Operation: operation,
			Msg:       fmt.Sprintf("%s=%d is millisecond-shaped; expected nanoseconds", field, value),
		}
	}
	return nil
}
