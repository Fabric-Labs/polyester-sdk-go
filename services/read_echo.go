package services

import (
	"fmt"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// echoed wraps the decoder of a read by ID: a reply that names another
// object than the one the request asked for breaks the response contract,
// however well formed it is, and returns a *errors.ResponseContractError.
// replied returns the identity the reply names and whether the reply holds
// the object at all; a reply without it is left to decodeFn. requested is
// the identity as the request sent it, in the form replied returns.
func echoed[Resp any, Out any](op, field, requested string, replied func(Resp) (string, bool), decodeFn func(Resp) (Out, error)) func(Resp) (Out, error) {
	return func(msg Resp) (Out, error) {
		out, err := decodeFn(msg)
		if err != nil {
			return out, err
		}
		if got, ok := replied(msg); ok && got != requested {
			var zero Out
			return zero, &sdkerrors.ResponseContractError{Operation: op, Msg: fmt.Sprintf("reply names %s %q, not the requested %q", field, got, requested)}
		}
		return out, nil
	}
}
