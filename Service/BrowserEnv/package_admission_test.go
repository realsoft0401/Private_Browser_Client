package BrowserEnv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	model "private_browser_client/Models/BrowserEnv"
)

func TestLoadAndValidateAtomicPackageAllowsMissingProxyConfigWhenProxyDisabled(t *testing.T) {
	envPath := createAtomicPackageForAdmissionTest(t, false, false)

	pkg, err := loadAndValidateAtomicPackage(envPath)
	if err != nil {
		t.Fatalf("expected disabled proxy package to pass admission without clash config: %v", err)
	}
	if pkg == nil {
		t.Fatal("expected atomic package result")
	}
	if pkg.Profile.Proxy.Enabled {
		t.Fatal("expected proxy to stay disabled")
	}
}

func TestLoadAndValidateAtomicPackageRejectsMissingProxyConfigWhenProxyEnabled(t *testing.T) {
	envPath := createAtomicPackageForAdmissionTest(t, true, false)

	_, err := loadAndValidateAtomicPackage(envPath)
	if err == nil {
		t.Fatal("expected enabled proxy package without clash config to fail admission")
	}
	if !strings.Contains(err.Error(), "环境包文件缺失 proxy/clash.yaml") {
		t.Fatalf("expected missing clash config error, got %v", err)
	}
}

func createAtomicPackageForAdmissionTest(t *testing.T, proxyEnabled bool, writeProxyConfig bool) string {
	t.Helper()

	envID := "906090001_tk_323733724883587072"
	userID := "906090001"
	rpaType := "tk"
	now := int64(1718188800)
	paths := defaultPackagePaths()
	envPath := filepath.Join(t.TempDir(), envID)

	if err := createEnvDirectories(envPath); err != nil {
		t.Fatalf("create env directories: %v", err)
	}

	identity := buildBindingIdentityFromFacts(envID, userID, rpaType)
	identityHash, err := buildJSONHash(identity)
	if err != nil {
		t.Fatalf("build identity hash: %v", err)
	}

	proxyType := ""
	if proxyEnabled {
		proxyType = "clash-verge"
	}

	profile := model.ProfileFile{
		SchemaVersion: model.SchemaVersion,
		EnvID:         envID,
		UserID:        userID,
		RPAType:       rpaType,
		SnowflakeID:   "323733724883587072",
		EnvSequence:   1,
		Name:          "admission-test",
		IdentityHash:  identityHash,
		Runtime: model.ProfileRuntime{
			Image:                "browser:test",
			ContainerUserDataDir: model.DefaultContainerUserDataDir,
			StartupURL:           model.DefaultStartupURL,
			EnableVNC:            true,
			ShmSize:              model.DefaultShmSize,
		},
		Environment: model.ProfileEnv{
			Timezone: "Asia/Shanghai",
			Language: "zh-CN",
			Screen: model.ProfileScreen{
				Width:  1280,
				Height: 720,
				Depth:  model.DefaultScreenDepth,
			},
		},
		Proxy: model.ProfileProxy{
			Enabled:    proxyEnabled,
			Type:       proxyType,
			ConfigPath: paths.ProxyConfig,
		},
		Paths: paths,
		Metadata: model.ProfileMetadata{
			Source:      "unit-test",
			Description: "package admission test fixture",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
	binding := model.BindingFile{
		ID:           "binding-test",
		Version:      1,
		IdentityHash: identityHash,
		Identity:     identity,
		Storage: model.BindingStorage{
			ContainerUserDataDir: model.DefaultContainerUserDataDir,
			HostUserDataDir:      paths.BrowserData,
		},
		SessionState: model.BindingSession{
			Platform:      rpaType,
			HasLoginState: false,
			Status:        "unknown",
		},
		Fingerprint: model.BindingFingerprint{
			SnapshotPath:      paths.FingerprintSnapshot,
			BackupPath:        paths.FingerprintBackup,
			RuntimeConfigPath: paths.FingerprintRuntimeConfig,
		},
		RuntimeProtection: model.RuntimeProtection{
			TimezoneStatus:     "pending",
			RiskStatus:         "pending",
			AvailabilityStatus: "pending",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	container := model.ContainerFile{
		EnvID:         envID,
		ContainerName: edgeBrowserContainerName(envID),
		Image:         "browser:test",
		Status:        "created",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	snapshot := model.FingerprintSnapshotFile{
		Raw: map[string]any{},
	}
	backup := model.FingerprintBackupFile{
		Available:          false,
		SourceSnapshotPath: paths.FingerprintSnapshot,
		Raw:                map[string]any{},
	}
	runtimeConfig := map[string]any{}
	proxyRuntime := model.ProxyRuntimeFile{
		Status: "pending",
		Drift:  false,
	}

	writeJSONForAdmissionTest(t, filepath.Join(envPath, "profile.json"), profile)
	writeJSONForAdmissionTest(t, filepath.Join(envPath, filepath.FromSlash(paths.Binding)), binding)
	writeJSONForAdmissionTest(t, filepath.Join(envPath, filepath.FromSlash(paths.Container)), container)
	writeJSONForAdmissionTest(t, filepath.Join(envPath, filepath.FromSlash(paths.FingerprintSnapshot)), snapshot)
	writeJSONForAdmissionTest(t, filepath.Join(envPath, filepath.FromSlash(paths.FingerprintBackup)), backup)
	writeJSONForAdmissionTest(t, filepath.Join(envPath, filepath.FromSlash(paths.FingerprintRuntimeConfig)), runtimeConfig)
	writeJSONForAdmissionTest(t, filepath.Join(envPath, filepath.FromSlash(paths.ProxyRuntime)), proxyRuntime)

	if writeProxyConfig {
		writeTextForAdmissionTest(t, filepath.Join(envPath, filepath.FromSlash(paths.ProxyConfig)), "mode: rule\nmixed-port: 7897\n")
	}

	return envPath
}

func writeJSONForAdmissionTest(t *testing.T, path string, value any) {
	t.Helper()
	if err := writeJSONFile(path, value); err != nil {
		t.Fatalf("write json fixture %s: %v", path, err)
	}
}

func writeTextForAdmissionTest(t *testing.T, path string, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create text fixture dir %s: %v", path, err)
	}
	if err := writeTextFile(path, value); err != nil {
		t.Fatalf("write text fixture %s: %v", path, err)
	}
}
