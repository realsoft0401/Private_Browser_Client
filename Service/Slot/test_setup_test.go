package Slot

import (
	"os"
	"path/filepath"
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
	code := m.Run()
	_ = sqliteInfra.Close()
	os.Exit(code)
}

func prepareTestProjectRoot() (string, error) {
	sourceRoot, err := findSourceProjectRoot()
	if err != nil {
		return "", err
	}
	tempRoot, err := os.MkdirTemp("", "private-browser-client-slot-test-*")
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
	if err = os.WriteFile(filepath.Join(tempRoot, "Settings", Settings.ConfigFileName), body, 0o644); err != nil {
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
