// Package useragent provides the stable HTTP User-Agent for Polyester SDK clients.
//
// Edge WAF rules have historically banned default library User-Agents
// (for example python-requests/*) with Cloudflare error 1010 before
// authentication runs. Language SDKs must send an explicit Polyester identity.
package useragent

import (
	"runtime/debug"
	"strings"
)

const product = "polyester-sdk-go"

// Header is the canonical User-Agent header name.
const Header = "User-Agent"

// String returns polyester-sdk-go[/version] using module build info when available.
func String() string {
	version := moduleVersion()
	if version == "" {
		return product
	}
	return product + "/" + version
}

func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return ""
	}
	version := strings.TrimSpace(info.Main.Version)
	if version == "" || version == "(devel)" {
		return ""
	}
	return version
}

// IsCloudflareBrowserBan reports whether body looks like Cloudflare error 1010.
func IsCloudflareBrowserBan(body string) bool {
	lower := strings.ToLower(body)
	if strings.Contains(lower, "error code: 1010") || strings.Contains(lower, "error code 1010") {
		return true
	}
	return strings.Contains(lower, "attention required") && strings.Contains(lower, "cloudflare")
}

// Cloudflare1010Message explains a WAF block that is not an API auth failure.
func Cloudflare1010Message() string {
	return "Request blocked by edge WAF (Cloudflare error 1010: browser signature banned). " +
		"This is not an API authentication failure. " +
		"Retry with User-Agent " + String() + " (set automatically by this SDK)."
}
