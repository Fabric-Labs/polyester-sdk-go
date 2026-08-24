package services

import "testing"

func TestStartSocialVerificationRequestForwardsAtHandle(t *testing.T) {
	req := startSocialVerificationRequest("twitter", "profile", "@alice")
	if req.GetHandle() != "@alice" {
		t.Fatalf("handle=%q", req.GetHandle())
	}
}
