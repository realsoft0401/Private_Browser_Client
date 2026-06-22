package BrowserEnv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqliteInfra "private_browser_client/Infrastructures/SQLite"
	"private_browser_client/Settings"
)

func TestMain(m *testing.M) {
	projectRoot, err := prepareTestProjectRoot()
	if err != nil {
		panic(err)
	}
	if err = Settings.Init(projectRoot); err != nil {
		panic(err)
	}
	if err = sqliteInfra.Init(); err != nil {
		panic(err)
	}
	portAvailabilityChecker = func(port int) error {
		_ = port
		return nil
	}
	browserEnvContainerStopper = func(slotID string, timeoutSeconds int) error {
		_, _ = slotID, timeoutSeconds
		return nil
	}
	browserStateFlusher = func(timeoutSeconds int) {
		_ = timeoutSeconds
	}
	code := m.Run()
	portAvailabilityChecker = ensureTCPPortAvailable
	browserEnvContainerStopper = gracefulStopBrowserEnvContainer
	browserStateFlusher = waitForBrowserStateFlush
	_ = sqliteInfra.Close()
	os.Exit(code)
}

func prepareTestProjectRoot() (string, error) {
	sourceRoot, err := findSourceProjectRoot()
	if err != nil {
		return "", err
	}
	tempRoot, err := os.MkdirTemp("", "private-browser-client-browser-env-test-*")
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Join(tempRoot, "Settings"), 0o755); err != nil {
		return "", err
	}
	body, err := os.ReadFile(filepath.Join(sourceRoot, "Settings", Settings.ConfigFileName))
	if err != nil {
		return "", err
	}
	configBody := string(body)
	replacements := map[string]string{
		"heartbeat:\n  enabled: true":     "heartbeat:\n  enabled: false",
		"discovery:\n  enabled: true":     "discovery:\n  enabled: false",
		"node_register:\n  enabled: true": "node_register:\n  enabled: false",
		"host_cdp_base_port: 9200":        "host_cdp_base_port: 38200",
		"host_vnc_base_port: 9100":        "host_vnc_base_port: 39200",
	}
	for oldValue, newValue := range replacements {
		configBody = strings.Replace(configBody, oldValue, newValue, 1)
	}
	if err = os.WriteFile(filepath.Join(tempRoot, "Settings", Settings.ConfigFileName), []byte(configBody), 0o644); err != nil {
		return "", err
	}
	return tempRoot, nil
}

func findSourceProjectRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(current, "Settings", Settings.ConfigFileName)); statErr == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}
