package services

import (
	"fmt"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// A failed construction-time catalog load never succeeds on the same client,
// so the SDK's own read polling must not read it as a transient failure, even
// when the load itself failed on transport or with a server error.
func TestReadUnavailableIgnoresCatalogHydrationFailure(t *testing.T) {
	for _, cause := range []error{
		&sdkerrors.TransportError{Msg: "dial tcp: connection refused"},
		&sdkerrors.ServerError{Msg: "unavailable", Status: 503},
	} {
		if !readUnavailable(cause) {
			t.Fatalf("readUnavailable(%T) = false; a bare transient failure is polled again", cause)
		}
		hydration := hydrationFailure(fmt.Errorf("catalog spot hydrate: %w", cause))
		if readUnavailable(hydration) {
			t.Errorf("readUnavailable(hydration error over %T) = true; the client never loads its catalogs again", cause)
		}
	}
}
