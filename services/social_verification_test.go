package services

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
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

// Telegram linking reuses social verification: Get("telegram") must ask for
// the Telegram verification, not the unspecified provider.
func TestSocialVerificationProviderSelectsTelegram(t *testing.T) {
	for _, name := range []string{"telegram", "TELEGRAM", "Telegram"} {
		if got := providerEnum(name); got != authv1.SocialProvider_TELEGRAM {
			t.Fatalf("providerEnum(%q) = %v, want TELEGRAM", name, got)
		}
	}
	req := startSocialVerificationRequest("telegram", "", "")
	if req.GetProvider() != authv1.SocialProvider_TELEGRAM || req.GetHandle() != "" {
		t.Fatalf("start request = %v", req)
	}
}

// A Telegram verification awaiting confirmation reads back with its status,
// display name and numeric Telegram user ID as a string.
func TestSocialVerificationReadsTelegramAwaitingConfirmation(t *testing.T) {
	data := decode.APIDataFromProtoMust(&authv1.GetSocialVerificationResponse{Verification: &authv1.SocialVerification{
		Provider:       authv1.SocialProvider_TELEGRAM,
		Status:         authv1.SocialVerificationStatus_STATUS_AWAITING_CONFIRMATION,
		Handle:         "alice_tg",
		DisplayName:    "Alice",
		ProviderUserId: "7654321098",
	}})
	verification, ok := data.Raw["verification"].(map[string]any)
	if !ok {
		t.Fatalf("raw=%v", data.Raw)
	}
	for field, want := range map[string]any{
		"provider": "TELEGRAM", "status": "STATUS_AWAITING_CONFIRMATION",
		"handle": "alice_tg", "display_name": "Alice", "provider_user_id": "7654321098",
	} {
		if verification[field] != want {
			t.Fatalf("%s = %#v, want %#v (verification=%v)", field, verification[field], want, verification)
		}
	}
}
