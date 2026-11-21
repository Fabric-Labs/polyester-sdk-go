package decode

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

// A strategy bot's key reads STRATEGY_BOT with its bot's public ID; every
// other key has no bot ID.
func TestAPIKeyReadsStrategyBot(t *testing.T) {
	botID := uint64(9223372036854775808)
	bot := ApiKeyMessageFromProto(&authv1.ApiKey{KeyId: "ak_1", Purpose: authv1.ApiKeyPurpose_STRATEGY_BOT, StrategyBotId: &botID})
	if bot.Purpose != "STRATEGY_BOT" || bot.StrategyBotID != codecs.FormatID(botID) {
		t.Fatalf("bot key = %+v", bot)
	}
	standard := ApiKeyMessageFromProto(&authv1.ApiKey{KeyId: "ak_2", Purpose: authv1.ApiKeyPurpose_STANDARD})
	if standard.StrategyBotID != "" {
		t.Fatalf("standard key = %+v", standard)
	}
}
