package services

import (
	"testing"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func TestStartSocialVerificationRequestForwardsAtHandle(t *testing.T) {
	req := startSocialVerificationRequest("twitter", "profile", "@alice")
	if req.GetHandle() != "@alice" {
		t.Fatalf("handle=%q", req.GetHandle())
	}
}

func TestStartSocialVerificationRequestAllowsDiscordWithoutHandle(t *testing.T) {
	req := startSocialVerificationRequest("discord", "", "")
	if req.GetProvider() != authv1.SocialProvider_DISCORD {
		t.Fatalf("provider=%v", req.GetProvider())
	}
	if req.GetHandle() != "" {
		t.Fatalf("discord handle must be omitted, got %q", req.GetHandle())
	}
	if req.GetMethod() != authv1.SocialVerificationMethod_METHOD_UNSPECIFIED {
		t.Fatalf("discord method should omit to server default, got %v", req.GetMethod())
	}
}
