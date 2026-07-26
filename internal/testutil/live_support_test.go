package testutil_test

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
)

func TestRouteUnavailable(t *testing.T) {
	if !testutil.RouteUnavailable(&sdkerrors.RouteNotFoundError{Procedure: "/x"}) {
		t.Fatal("expected route not found")
	}
	if !testutil.RouteUnavailable(&sdkerrors.APIError{Code: "unimplemented"}) {
		t.Fatal("expected unimplemented api error")
	}
	if testutil.RouteUnavailable(errors.New("other")) {
		t.Fatal("unexpected route unavailable")
	}
}

func TestJWTSessionOnly(t *testing.T) {
	if !testutil.JWTSessionOnly(&sdkerrors.AuthError{Msg: "missing Authorization header"}) {
		t.Fatal("expected jwt-only auth error")
	}
	if testutil.JWTSessionOnly(&sdkerrors.AuthError{Msg: "invalid signature"}) {
		t.Fatal("unexpected jwt-only")
	}
	// Permission-denied is classified separately (F-24).
	if testutil.JWTSessionOnly(&sdkerrors.AuthError{Msg: "permission_denied"}) {
		t.Fatal("permission_denied should not classify as jwt-only")
	}
}

func TestIsPermissionDenied(t *testing.T) {
	if !testutil.IsPermissionDenied(&sdkerrors.AuthError{Msg: "permission_denied: missing transfer:read"}) {
		t.Fatal("expected permission denied auth error")
	}
	if !testutil.IsPermissionDenied(&sdkerrors.APIError{Code: "permission_denied", Msg: "missing address-book"}) {
		t.Fatal("expected permission denied api error")
	}
	if testutil.IsPermissionDenied(&sdkerrors.AuthError{Msg: "missing Authorization header"}) {
		t.Fatal("unexpected permission denied")
	}
}

func TestDevnetProtoMismatch(t *testing.T) {
	if !testutil.DevnetProtoMismatch(&sdkerrors.ServerError{Msg: "internal error: proto decode"}) {
		t.Fatal("expected proto mismatch")
	}
}
