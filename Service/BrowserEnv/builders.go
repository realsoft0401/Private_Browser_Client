package BrowserEnv

import (
	"encoding/json"
	"strconv"
	"strings"

	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Settings"
)

// defaultManifestPaths 返回环境包内部文件的标准相对路径。
//
// 这些路径不能写成机器绝对路径，因为环境包后续要支持打包上传、下载到另一台设备后继续运行。
func defaultManifestPaths() model.ManifestPaths {
	return model.ManifestPaths{
		Profile:                  "profile.json",
		Binding:                  "binding.json",
		Container:                "container.json",
		BrowserData:              "browser-data/profile",
		FingerprintSnapshot:      "fingerprint/snapshot.json",
		FingerprintBackup:        "fingerprint/backup.json",
		FingerprintRuntimeConfig: "fingerprint/runtime-config.json",
		ProxyConfig:              "proxy/clash.yaml",
		ProxyRuntime:             "proxy/proxy-runtime.json",
		Logs:                     "logs",
	}
}

func buildEnvID(userID string, rpaType string, snowflakeID string) string {
	return userID + "_" + rpaType + "_" + snowflakeID
}

// buildPackageFiles 组装本次创建要写入环境包的全部文件模型。
//
// 它只做“从已归一化请求 + 生成上下文 -> 文件模型”的转换，
// 不负责写磁盘，方便后续单独测试 hash、端口和文件内容是否正确。
func buildPackageFiles(ctx *createContext) (envPackageFiles, error) {
	manifest := buildManifestFile(ctx)
	profile := buildProfileFile(ctx)
	binding := buildBindingFile(ctx)
	container := buildContainerFile(ctx)
	snapshot := buildFingerprintSnapshotFile()
	backup, runtimeConfig, err := buildFingerprintFiles(ctx.Param, ctx.Paths, ctx.Now)
	if err != nil {
		return envPackageFiles{}, err
	}

	return envPackageFiles{
		Manifest:      manifest,
		Profile:       profile,
		Binding:       binding,
		Container:     container,
		Snapshot:      snapshot,
		Backup:        backup,
		RuntimeConfig: runtimeConfig,
		ProxyRuntime:  buildProxyRuntimeFile(),
		ProxyConfig:   ctx.Param.Proxy.Config,
	}, nil
}

func buildManifestFile(ctx *createContext) model.ManifestFile {
	return model.ManifestFile{
		SchemaVersion: model.SchemaVersion,
		EnvID:         ctx.EnvID,
		UserID:        ctx.Param.UserID,
		RPAType:       ctx.Param.RPAType,
		SnowflakeID:   ctx.SnowflakeID,
		EnvSequence:   ctx.EnvSequence,
		Paths:         ctx.Paths,
		LastRuntime:   model.ManifestLastRuntime{},
		CreatedAt:     ctx.Now,
		UpdatedAt:     ctx.Now,
	}
}

func buildProfileFile(ctx *createContext) model.ProfileFile {
	proxyEnabled := false
	if ctx.Param.Proxy.Enabled != nil {
		proxyEnabled = *ctx.Param.Proxy.Enabled
	}
	return model.ProfileFile{
		EnvID:       ctx.EnvID,
		EnvSequence: ctx.EnvSequence,
		Name:        ctx.Param.Name,
		RPAType:     ctx.Param.RPAType,
		Runtime: model.ProfileRuntime{
			Image:                ctx.Param.Runtime.Image,
			ContainerUserDataDir: model.DefaultContainerUserDataDir,
			StartupURL:           ctx.Param.Runtime.StartupURL,
			EnableVNC:            true,
			ShmSize:              ctx.Param.Runtime.ShmSize,
		},
		Environment: model.ProfileEnv{
			Timezone: ctx.Param.Environment.Timezone,
			Language: ctx.Param.Environment.Language,
			Screen: model.ProfileScreen{
				Width:  ctx.Param.Environment.Screen.Width,
				Height: ctx.Param.Environment.Screen.Height,
				Depth:  ctx.Param.Environment.Screen.Depth,
			},
		},
		Ports: ctx.Ports,
		Proxy: model.ProfileProxy{
			Enabled:    proxyEnabled,
			Type:       ctx.Param.Proxy.Type,
			ConfigPath: ctx.Paths.ProxyConfig,
		},
		Metadata: model.ProfileMetadata{
			Source:      ctx.Param.Metadata.Source,
			Description: ctx.Param.Metadata.Description,
			CreatedAt:   ctx.Now,
			UpdatedAt:   ctx.Now,
		},
	}
}

func buildBindingFile(ctx *createContext) model.BindingFile {
	return model.BindingFile{
		ID:           ctx.BindingID,
		Version:      1,
		Locked:       false,
		IdentityHash: ctx.IdentityHash,
		ConfigHash:   ctx.ConfigHash,
		Identity:     ctx.Identity,
		Storage: model.BindingStorage{
			ContainerUserDataDir: model.DefaultContainerUserDataDir,
			HostUserDataDir:      ctx.Paths.BrowserData,
		},
		SessionState: model.BindingSession{
			Platform:      ctx.Param.RPAType,
			HasLoginState: false,
			Status:        "unknown",
		},
		Fingerprint: model.BindingFingerprint{
			SnapshotPath:      ctx.Paths.FingerprintSnapshot,
			BackupPath:        ctx.Paths.FingerprintBackup,
			RuntimeConfigPath: ctx.Paths.FingerprintRuntimeConfig,
			Restored:          false,
		},
		RuntimeProtection: model.RuntimeProtection{
			TimezoneStatus: "pending",
		},
		CreatedAt: ctx.Now,
		UpdatedAt: ctx.Now,
	}
}

func buildContainerFile(ctx *createContext) model.ContainerFile {
	containerName := "bv-" + strings.ReplaceAll(ctx.EnvID, "_", "-")
	return model.ContainerFile{
		EnvID:         ctx.EnvID,
		ContainerName: containerName,
		Image:         ctx.Param.Runtime.Image,
		Status:        "created",
		Ports:         ctx.Ports,
		Docker: model.ContainerDocker{
			APIURL: Settings.Conf.DockerConfig.APIURL,
		},
		Labels: map[string]string{
			"bv.project":       "private-browser-client",
			"bv.role":          "browser-env",
			"bv.envId":         ctx.EnvID,
			"bv.userId":        ctx.Param.UserID,
			"bv.rpaType":       ctx.Param.RPAType,
			"bv.schemaVersion": strconv.Itoa(model.SchemaVersion),
		},
		CreatedAt: ctx.Now,
		UpdatedAt: ctx.Now,
	}
}

func buildFingerprintSnapshotFile() model.FingerprintSnapshotFile {
	return model.FingerprintSnapshotFile{
		OK:        false,
		Source:    "",
		TargetURL: "",
		PageURL:   "",
		Title:     "",
		Score:     model.FingerprintScore{},
		Raw:       map[string]any{},
	}
}

// buildFingerprintFiles 初始化正式指纹备份和容器注入配置。
//
// 第一版创建环境包时通常没有正式指纹备份，所以 backup.json 写 available=false，
// runtime-config.json 写空对象。只有服务端明确带入可恢复指纹时，才生成可注入配置。
func buildFingerprintFiles(param *model.CreateBrowserEnvRequest, paths model.ManifestPaths, now int64) (model.FingerprintBackupFile, any, error) {
	backupReq := (*model.CreateFingerprintBackupRequest)(nil)
	if param.Fingerprint != nil {
		backupReq = param.Fingerprint.Backup
	}
	if backupReq == nil || backupReq.Fingerprint == nil {
		return model.FingerprintBackupFile{
			Available:          false,
			SourceSnapshotPath: paths.FingerprintSnapshot,
			Raw:                map[string]any{},
		}, map[string]any{}, nil
	}

	raw := any(map[string]any{})
	if len(backupReq.Raw) > 0 {
		if err := json.Unmarshal(backupReq.Raw, &raw); err != nil {
			return model.FingerprintBackupFile{}, nil, invalidError("fingerprint.backup.raw 必须是合法 JSON")
		}
	}
	return model.FingerprintBackupFile{
		Available:          true,
		SavedAt:            &now,
		SourceSnapshotPath: paths.FingerprintSnapshot,
		Fingerprint:        backupReq.Fingerprint,
		Raw:                raw,
	}, backupReq.Fingerprint, nil
}

func buildProxyRuntimeFile() model.ProxyRuntimeFile {
	source := "container-probe"
	return model.ProxyRuntimeFile{
		Source: &source,
		Status: "pending",
		Drift:  false,
	}
}
