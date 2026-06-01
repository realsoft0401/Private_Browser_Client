package BrowserEnv

import (
	"strings"
	"testing"
)

func TestDetectClashTunEnabled(t *testing.T) {
	config := `
mode: rule
tun:
  enable: true
  stack: system
`
	enabled, err := detectClashTunEnabled(config)
	if err != nil {
		t.Fatalf("detectClashTunEnabled returned error: %v", err)
	}
	if !enabled {
		t.Fatal("expected tun.enable=true to require TUN")
	}
}

func TestDetectClashTunEnabledIgnoresComments(t *testing.T) {
	config := `
mode: rule
# tun:
#   enable: true
dns:
  enable: true
`
	enabled, err := detectClashTunEnabled(config)
	if err != nil {
		t.Fatalf("detectClashTunEnabled returned error: %v", err)
	}
	if enabled {
		t.Fatal("commented tun.enable=true must not require TUN")
	}
}

func TestDetectClashTunEnabledFalseWhenMissing(t *testing.T) {
	enabled, err := detectClashTunEnabled("mode: global\nmixed-port: 7897\n")
	if err != nil {
		t.Fatalf("detectClashTunEnabled returned error: %v", err)
	}
	if enabled {
		t.Fatal("missing tun block must not require TUN")
	}
}

func TestDetectClashTunEnabledInvalidYAML(t *testing.T) {
	_, err := detectClashTunEnabled("tun:\n  enable: [true\n")
	if err == nil {
		t.Fatal("expected invalid YAML to return an error")
	}
	if _, ok := IsBusinessError(err); !ok {
		t.Fatalf("expected business error, got %T", err)
	}
}

func TestDisableClashTunForRuntime(t *testing.T) {
	config := `
mode: global
mixed-port: 7897
tun:
  enable: true
  stack: system
proxies:
  - name: outNode
    type: http
`
	updated, err := disableClashTunForRuntime(config)
	if err != nil {
		t.Fatalf("disableClashTunForRuntime returned error: %v", err)
	}
	enabled, err := detectClashTunEnabled(updated)
	if err != nil {
		t.Fatalf("detect updated config returned error: %v", err)
	}
	if enabled {
		t.Fatalf("expected runtime config tun.enable=false, got:\n%s", updated)
	}
	if !strings.Contains(updated, "mixed-port: 7897") || !strings.Contains(updated, "outNode") {
		t.Fatalf("expected non-tun config to be preserved, got:\n%s", updated)
	}
}
