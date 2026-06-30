package testutil

import (
	"os"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
)

// LiveClientFromEnv returns a client when POLYESTER_API_KEY_* env vars are set.
func LiveClientFromEnv() (*polyester.Client, bool, error) {
	loadDotEnv()
	if strings.TrimSpace(os.Getenv(auth.APIKeyIDEnv)) == "" {
		return nil, false, nil
	}
	if strings.TrimSpace(os.Getenv(auth.APIPrivateKeyEnv)) == "" {
		return nil, false, nil
	}
	client, err := polyester.FromEnv(func(c *polyester.Config) {
		c.HydrateCatalogs = true
		if apiURL := strings.TrimSpace(os.Getenv("POLYESTER_API_URL")); apiURL != "" {
			c.APIURL = apiURL
		}
		if wsURL := strings.TrimSpace(os.Getenv("POLYESTER_WS_URL")); wsURL != "" {
			c.WSURL = wsURL
		}
	})
	if err != nil {
		return nil, false, err
	}
	return client, true, nil
}
