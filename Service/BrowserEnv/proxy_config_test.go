package BrowserEnv

import (
	"strings"
	"testing"

	model "private_browser_client/Models/BrowserEnv"
)

func testStringPtr(value string) *string {
	return &value
}

func testBoolPtr(value bool) *bool {
	return &value
}

func TestNormalizeProxyUpdateModeOnlyUpdatesExistingConfig(t *testing.T) {
	pkg := &proxyConfigPackage{
		Profile: model.ProfileFile{
			Proxy: model.ProfileProxy{
				Enabled:    true,
				Type:       "clash-verge",
				ConfigPath: "proxy/clash.yaml",
			},
		},
		ProxyConfig: "mode: rule\nmixed-port: 7897\nrules:\n  - MATCH,relay\n",
	}
	param := &model.UpdateBrowserEnvProxyRequest{
		Mode: testStringPtr("global"),
	}

	normalized, err := normalizeProxyUpdate(pkg, param)
	if err != nil {
		t.Fatalf("normalizeProxyUpdate returned error: %v", err)
	}
	if !normalized.Changed {
		t.Fatal("expected mode-only update to mark config changed")
	}
	if !strings.Contains(normalized.Config, "mode: global") {
		t.Fatalf("expected config mode to become global, got:\n%s", normalized.Config)
	}
	if strings.Contains(normalized.Config, "mode: rule") {
		t.Fatalf("expected old mode to be replaced, got:\n%s", normalized.Config)
	}
}

func TestNormalizeProxyUpdateDisableIgnoresModeWhenConfigEmpty(t *testing.T) {
	pkg := &proxyConfigPackage{
		Profile: model.ProfileFile{
			Proxy: model.ProfileProxy{
				Enabled: true,
				Type:    "clash-verge",
			},
		},
	}
	param := &model.UpdateBrowserEnvProxyRequest{
		Enabled: testBoolPtr(false),
		Mode:    testStringPtr("global"),
	}

	normalized, err := normalizeProxyUpdate(pkg, param)
	if err != nil {
		t.Fatalf("disable proxy with mode should not require existing config: %v", err)
	}
	if !normalized.Changed {
		t.Fatal("expected disabling proxy to be a real change")
	}
	if normalized.Enabled {
		t.Fatal("expected proxy to be disabled")
	}
	if normalized.Type != "" || normalized.Config != "" {
		t.Fatalf("expected disabled proxy to clear type/config, got type=%q config=%q", normalized.Type, normalized.Config)
	}
}
