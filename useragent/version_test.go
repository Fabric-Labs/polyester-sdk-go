package useragent

import (
	"runtime/debug"
	"testing"
)

func TestSDKVersionReadsTheSDKDependencyNotTheApplication(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Path: "example.com/trading-bot", Version: "v3.1.0"},
		Deps: []*debug.Module{
			{Path: "google.golang.org/protobuf", Version: "v1.36.11"},
			{Path: modulePath, Version: "v0.2.0"},
		},
	}
	if got := sdkVersion(info); got != "v0.2.0" {
		t.Fatalf("sdkVersion=%q want v0.2.0", got)
	}
}

func TestSDKVersionOmitsUnknownVersions(t *testing.T) {
	for name, info := range map[string]*debug.BuildInfo{
		"nil":             nil,
		"sdk not linked":  {Main: debug.Module{Path: "example.com/trading-bot", Version: "v3.1.0"}},
		"sdk main devel":  {Main: debug.Module{Path: modulePath, Version: "(devel)"}},
		"sdk main empty":  {Main: debug.Module{Path: modulePath}},
		"sdk dep no vers": {Main: debug.Module{Path: "example.com/bot"}, Deps: []*debug.Module{{Path: modulePath}}},
	} {
		if got := sdkVersion(info); got != "" {
			t.Errorf("%s: sdkVersion=%q want empty", name, got)
		}
	}
}

func TestSDKVersionReadsTheSDKAsMainModule(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: "v0.2.0"}}
	if got := sdkVersion(info); got != "v0.2.0" {
		t.Fatalf("sdkVersion=%q want v0.2.0", got)
	}
}
