package BrowserEnv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Settings"
)

var (
	createEnvMu = sync.Mutex{}
	idGen       = newSnowflakeGenerator(1)
	// portAvailabilityChecker 默认走真实 TCP 监听探测。
	//
	// 设计来源：
	// - 正式运行时，create/import 仍然需要明确避开已被占用的宿主端口；
	// - 但测试和受限执行环境里，`net.Listen` 可能被平台直接禁止，导致“探测能力”本身成为噪音；
	// - 因此这里保留真实默认值，同时允许测试替换成受控 stub，避免把环境限制误判成业务失败。
	portAvailabilityChecker = ensureTCPPortAvailable
)

// createContext 集中保存一次 create-browser-env 生成的派生事实。
type createContext struct {
	Param           *model.CreateBrowserEnvRequest
	Now             int64
	SnowflakeID     string
	EnvID           string
	BindingID       string
	EnvSequence     int
	Ports           model.BrowserEnvPorts
	Paths           model.PackagePaths
	RelativeEnvPath string
	AbsoluteEnvPath string
	Identity        model.BindingIdentity
	IdentityHash    string
}

type snowflakeGenerator struct {
	mu       sync.Mutex
	epochMS  int64
	workerID int64
	lastMS   int64
	sequence int64
}

func newSnowflakeGenerator(workerID int64) *snowflakeGenerator {
	return &snowflakeGenerator{
		epochMS:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
		workerID: workerID & 0x3ff,
	}
}

func (g *snowflakeGenerator) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	nowMS := time.Now().UnixMilli()
	if nowMS < g.lastMS {
		nowMS = g.lastMS
	}
	if nowMS == g.lastMS {
		g.sequence = (g.sequence + 1) & 0xfff
		if g.sequence == 0 {
			for nowMS <= g.lastMS {
				nowMS = time.Now().UnixMilli()
			}
		}
	} else {
		g.sequence = 0
	}
	g.lastMS = nowMS
	id := ((nowMS - g.epochMS) << 22) | (g.workerID << 12) | g.sequence
	return strconv.FormatInt(id, 10)
}

func defaultPackagePaths() model.PackagePaths {
	return model.PackagePaths{
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

func newCreateContext(param *model.CreateBrowserEnvRequest) (*createContext, error) {
	now := time.Now().Unix()
	snowflakeID := idGen.Next()
	envID := buildEnvID(param.UserID, param.RPAType, snowflakeID)
	envSequence, ports, err := nextAvailableEnvSequenceAndPorts()
	if err != nil {
		return nil, internalError(err.Error())
	}
	relativeEnvPath := filepath.ToSlash(filepath.Join("data", "browser-envs", "users", param.UserID, param.RPAType, envID))
	if Settings.Conf.ProjectRoot == "" {
		return nil, internalError("project root 不能为空")
	}
	ctx := &createContext{
		Param:           param,
		Now:             now,
		SnowflakeID:     snowflakeID,
		EnvID:           envID,
		BindingID:       fmt.Sprintf("binding-%s-%s-%s", param.UserID, param.RPAType, snowflakeID),
		EnvSequence:     envSequence,
		Ports:           ports,
		Paths:           defaultPackagePaths(),
		RelativeEnvPath: relativeEnvPath,
		AbsoluteEnvPath: filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(relativeEnvPath)),
	}
	ctx.Identity = buildBindingIdentity(ctx.EnvID, param.UserID, param.RPAType)
	ctx.IdentityHash, err = buildJSONHash(ctx.Identity)
	if err != nil {
		return nil, internalError(fmt.Sprintf("计算 identityHash 失败: %v", err))
	}
	return ctx, nil
}

func buildEnvID(userID string, rpaType string, snowflakeID string) string {
	return userID + "_" + rpaType + "_" + snowflakeID
}

func buildBindingIdentity(envID string, userID string, rpaType string) model.BindingIdentity {
	return model.BindingIdentity{
		EnvID:   envID,
		UserID:  userID,
		RPAType: rpaType,
	}
}

func buildJSONHash(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func nextAvailableEnvSequenceAndPorts() (int, model.BrowserEnvPorts, error) {
	start, err := nextEnvSequence()
	if err != nil {
		return 0, model.BrowserEnvPorts{}, err
	}
	cdpBasePort := 8100
	vncBasePort := 9100
	if Settings.Conf.SlotRuntimeConfig != nil {
		if Settings.Conf.SlotRuntimeConfig.HostCDPBasePort > 0 {
			cdpBasePort = Settings.Conf.SlotRuntimeConfig.HostCDPBasePort
		}
		if Settings.Conf.SlotRuntimeConfig.HostVNCBasePort > 0 {
			vncBasePort = Settings.Conf.SlotRuntimeConfig.HostVNCBasePort
		}
	}
	for sequence := start; sequence < start+10000; sequence++ {
		// 这里必须遵守配置里的宿主端口基线，而不是回退到旧代码里的写死端口。
		//
		// 设计来源：
		// - 新 Client 已经把 slot/browser-env 的宿主端口策略收口到 Settings；
		// - 如果 create 仍然写死 8100/9100，测试环境和实际部署一旦改了基线端口，就会出现“配置写了但实现没用”的假一致；
		// - 这次 BrowserEnv 测试暴露出的端口冲突，根因就是这里没有真正吃配置。
		ports := model.BrowserEnvPorts{CDP: cdpBasePort + sequence, VNC: vncBasePort + sequence}
		if portAvailabilityChecker(ports.CDP) == nil && portAvailabilityChecker(ports.VNC) == nil {
			return sequence, ports, nil
		}
	}
	return 0, model.BrowserEnvPorts{}, fmt.Errorf("无法分配可用 CDP/VNC 端口")
}

// nextEnvSequence 从本机已有 profile.json 扫描下一个序号。
//
// 当前先沿用 old 的简单方式：以文件事实为准。
// 这样即使 SQLite 临时损坏，只要环境包目录还在，序号也不会从 1 重新撞上。
func nextEnvSequence() (int, error) {
	root := filepath.Join(Settings.Conf.ProjectRoot, "data", "browser-envs")
	maxSequence := 0
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return 1, nil
	} else if err != nil {
		return 0, fmt.Errorf("读取 browser-envs 根目录失败: %w", err)
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "profile.json" {
			return nil
		}
		var payload struct {
			EnvSequence int `json:"envSequence"`
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		if payload.EnvSequence > maxSequence {
			maxSequence = payload.EnvSequence
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("扫描 envSequence 失败: %w", err)
	}
	return maxSequence + 1, nil
}

func ensureTCPPortAvailable(port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("端口 %d 不可用: %w", port, err)
	}
	_ = listener.Close()
	return nil
}

func ensureEnvPathAvailable(envPath string) error {
	if _, err := os.Stat(envPath); err == nil {
		return conflictError("envPath 已存在但不是本次创建的新环境包")
	} else if !os.IsNotExist(err) {
		return internalError(fmt.Sprintf("检查 envPath 失败: %v", err))
	}
	return nil
}

func cleanupPartialEnvPackage(envPath string, created *bool) {
	if created == nil || *created {
		return
	}
	_ = os.RemoveAll(envPath)
}

func buildPackageFiles(ctx *createContext) envPackageFiles {
	proxyEnabled := ctx.Param.Proxy.Enabled != nil && *ctx.Param.Proxy.Enabled
	source := "container-probe"
	return envPackageFiles{
		Profile: model.ProfileFile{
			SchemaVersion: model.SchemaVersion,
			EnvID:         ctx.EnvID,
			UserID:        ctx.Param.UserID,
			RPAType:       ctx.Param.RPAType,
			SnowflakeID:   ctx.SnowflakeID,
			EnvSequence:   ctx.EnvSequence,
			Name:          ctx.Param.Name,
			IdentityHash:  ctx.IdentityHash,
			Runtime: model.ProfileRuntime{
				Image:                ctx.Param.Runtime.Image,
				ContainerUserDataDir: model.DefaultContainerUserDataDir,
				StartupURL:           ctx.Param.Runtime.StartupURL,
				EnableVNC:            true,
				ShmSize:              ctx.Param.Runtime.ShmSize,
			},
			Environment: model.ProfileEnvironment{
				Timezone: ctx.Param.Environment.Timezone,
				Language: model.FixedLanguage,
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
			Paths:       ctx.Paths,
			Metadata:    model.ProfileMetadata{Source: "api", CreatedAt: ctx.Now, UpdatedAt: ctx.Now},
			LastRuntime: model.PackageLastRuntime{},
		},
		Binding: model.BindingFile{
			ID:           ctx.BindingID,
			Version:      1,
			Locked:       false,
			IdentityHash: ctx.IdentityHash,
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
				TimezoneStatus:     "pending",
				RiskStatus:         "pending",
				AvailabilityStatus: "pending",
			},
			CreatedAt: ctx.Now,
			UpdatedAt: ctx.Now,
		},
		Container: model.ContainerFile{
			EnvID:         ctx.EnvID,
			ContainerName: edgeBrowserContainerName(ctx.EnvID),
			Image:         ctx.Param.Runtime.Image,
			Status:        model.ContainerStatusCreated,
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
		},
		Snapshot: model.FingerprintSnapshotFile{
			OK:        false,
			TargetURL: "",
			PageURL:   "",
			Title:     "",
			Score:     model.FingerprintScore{},
			Raw:       map[string]any{},
		},
		Backup: model.FingerprintBackupFile{
			Available:          false,
			SourceSnapshotPath: ctx.Paths.FingerprintSnapshot,
			Raw:                map[string]any{},
		},
		ProxyRuntime: model.ProxyRuntimeFile{
			Source: &source,
			Status: "pending",
			Drift:  false,
		},
		ProxyConfig: ctx.Param.Proxy.Config,
	}
}

func buildBrowserEnvIndex(ctx *createContext, files envPackageFiles) *model.BrowserEnvIndex {
	containerName := files.Container.ContainerName
	return &model.BrowserEnvIndex{
		EnvID:               ctx.EnvID,
		UserID:              ctx.Param.UserID,
		RPAType:             ctx.Param.RPAType,
		Name:                ctx.Param.Name,
		EnvSequence:         ctx.EnvSequence,
		CDPPort:             ctx.Ports.CDP,
		VNCPort:             ctx.Ports.VNC,
		EnvPath:             ctx.RelativeEnvPath,
		Status:              model.BrowserEnvStatusCreated,
		ContainerName:       &containerName,
		ContainerStatus:     model.ContainerStatusMissing,
		MonitorStatus:       model.MonitorStatusUnknown,
		FingerprintRestored: false,
		HasBrowserData:      true,
		CreatedAt:           ctx.Now,
		UpdatedAt:           ctx.Now,
	}
}

func buildCreateResponse(ctx *createContext) *model.CreateBrowserEnvResponse {
	return &model.CreateBrowserEnvResponse{
		EnvID:       ctx.EnvID,
		UserID:      ctx.Param.UserID,
		RPAType:     ctx.Param.RPAType,
		EnvSequence: ctx.EnvSequence,
		Ports:       ctx.Ports,
		EnvPath:     ctx.RelativeEnvPath,
		Files: map[string]string{
			"profile":   ctx.Paths.Profile,
			"binding":   ctx.Paths.Binding,
			"container": ctx.Paths.Container,
		},
		IdentityHash: ctx.IdentityHash,
		CreatedAt:    ctx.Now,
	}
}

func edgeBrowserContainerName(envID string) string {
	trimmed := strings.TrimSpace(envID)
	parts := strings.Split(trimmed, "_")
	if len(parts) == 3 {
		return "private-browser-edge-" + parts[0] + "-" + parts[1] + "-" + parts[2]
	}
	return "private-browser-edge-" + strings.ReplaceAll(trimmed, "_", "-")
}
