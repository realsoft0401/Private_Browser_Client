package BrowserEnv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Settings"
)

// GetBrowserEnvDetail 返回单个环境包的结构化详情。
//
// 设计来源：
// - 列表接口只给前端展示摘要，不足以支撑“环境详情页”和后续代理重新配置；
// - 用户要求先做详情接口，同时提醒后面还要有代理修改 API，所以详情必须把 proxy 的路径、hash、runtime 摘要暴露出来；
// - 环境包的主事实仍是 SQLite 索引 + 本地文件，不做 Docker 实时探测。
//
// 职责边界：
// - 读取 browser_envs 索引，再按 envPath 读取 manifest/profile/binding/container/fingerprint/proxy runtime；
// - 返回代理和指纹摘要，但不返回 proxy/clash.yaml 正文或 fingerprint raw；
// - 只做轻量一致性检查，不修改文件，不启动容器，不刷新 Docker 状态。
func (s *Service) GetBrowserEnvDetail(envID string, httpBase string, wsBase string) (*model.BrowserEnvDetailResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}

	index, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}
	attachRunningVNCLinks([]*model.BrowserEnvIndex{index}, httpBase, wsBase)

	detail, err := loadBrowserEnvDetail(index)
	if err != nil {
		return nil, internalError(err.Error())
	}
	detail.Index = index
	if index.VNCURL != "" || index.VNCWSURL != "" || index.WebVNCURL != "" {
		detail.VNC = &model.BrowserEnvDetailVNCConnection{
			VNCURL:    index.VNCURL,
			VNCWSURL:  index.VNCWSURL,
			WebVNCURL: index.WebVNCURL,
		}
	}
	return detail, nil
}

// loadBrowserEnvDetail 从环境包目录读取详情文件。
//
// 这里不复用 loadRunPackage，是因为 run 会创建 browser-data/profile 并强校验 identityHash；
// 详情页需要先把事实展示出来，即使某些文件缺失，也应该通过 consistency.errors 告诉前端问题在哪里。
func loadBrowserEnvDetail(index *model.BrowserEnvIndex) (*model.BrowserEnvDetailResponse, error) {
	if index == nil {
		return nil, fmt.Errorf("环境包索引不能为空")
	}
	if Settings.Conf.ProjectRoot == "" {
		return nil, fmt.Errorf("project root 不能为空")
	}

	envPath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(index.EnvPath))
	if stat, err := os.Stat(envPath); err != nil {
		return nil, fmt.Errorf("环境包目录不存在: %w", err)
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("环境包路径不是目录")
	}

	var manifest model.ManifestFile
	if err := readJSONFile(filepath.Join(envPath, "manifest.json"), &manifest); err != nil {
		return nil, err
	}
	var profile model.ProfileFile
	if err := readPackageJSON(envPath, manifest.Paths.Profile, &profile); err != nil {
		return nil, err
	}
	var binding model.BindingFile
	if err := readPackageJSON(envPath, manifest.Paths.Binding, &binding); err != nil {
		return nil, err
	}
	var container model.ContainerFile
	if err := readPackageJSON(envPath, manifest.Paths.Container, &container); err != nil {
		return nil, err
	}

	proxyDetail, proxyConfigText, proxyConfigExists, err := readProxyDetail(envPath, profile, manifest.Paths)
	if err != nil {
		return nil, err
	}
	fingerprintDetail, err := readFingerprintDetail(envPath, manifest.Paths)
	if err != nil {
		return nil, err
	}

	consistency := buildDetailConsistency(index, envPath, manifest, profile, binding, proxyConfigText, proxyConfigExists)
	return &model.BrowserEnvDetailResponse{
		Manifest:    manifest,
		Profile:     profile,
		Binding:     toBindingDetail(binding),
		Container:   container,
		Proxy:       proxyDetail,
		Fingerprint: fingerprintDetail,
		Consistency: consistency,
		Files:       buildDetailFiles(manifest.Paths),
	}, nil
}

// readPackageJSON 在环境包根目录下安全读取相对路径 JSON。
//
// manifest 里的路径来自本机生成，但后续环境包可导入，不能默认信任路径不会越界。
func readPackageJSON(envPath string, relativePath string, target any) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	return readJSONFile(path, target)
}

// safePackagePath 把环境包内相对路径转换成绝对路径，并挡住路径逃逸。
//
// 这个保护是为了后续导入环境包做准备：manifest 里的相对路径不能通过 ../ 读到环境包外部。
func safePackagePath(envPath string, relativePath string) (string, error) {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return "", fmt.Errorf("环境包文件路径不能为空")
	}
	cleanEnvPath, err := filepath.Abs(envPath)
	if err != nil {
		return "", fmt.Errorf("解析环境包目录失败: %w", err)
	}
	targetPath, err := filepath.Abs(filepath.Join(cleanEnvPath, filepath.FromSlash(relativePath)))
	if err != nil {
		return "", fmt.Errorf("解析环境包文件路径失败: %w", err)
	}
	if targetPath != cleanEnvPath && !strings.HasPrefix(targetPath, cleanEnvPath+string(filepath.Separator)) {
		return "", fmt.Errorf("环境包文件路径越界: %s", relativePath)
	}
	return targetPath, nil
}

// readProxyDetail 读取代理配置摘要。
//
// 代理正文只用于计算 hash 和 size，不进入响应体；代理修改要走专门 API，不能让详情接口承担编辑职责。
func readProxyDetail(envPath string, profile model.ProfileFile, paths model.ManifestPaths) (model.BrowserEnvProxyDetail, string, bool, error) {
	detail := model.BrowserEnvProxyDetail{
		Enabled:    profile.Proxy.Enabled,
		Type:       profile.Proxy.Type,
		ConfigPath: profile.Proxy.ConfigPath,
	}
	configPath := profile.Proxy.ConfigPath
	if configPath == "" {
		configPath = paths.ProxyConfig
	}
	proxyConfigText := ""
	proxyConfigExists := false
	if configPath != "" {
		absoluteConfigPath, err := safePackagePath(envPath, configPath)
		if err != nil {
			return detail, "", false, err
		}
		bytes, err := os.ReadFile(absoluteConfigPath)
		if err == nil {
			proxyConfigText = string(bytes)
			proxyConfigExists = true
			detail.ConfigPath = configPath
			detail.ConfigHash = buildTextHash(proxyConfigText)
			detail.ConfigSizeBytes = len(bytes)
			detail.Mode = extractClashMode(proxyConfigText)
		} else if !os.IsNotExist(err) {
			return detail, "", false, fmt.Errorf("读取代理配置摘要失败: %w", err)
		}
	}
	if paths.ProxyRuntime != "" {
		if err := readOptionalPackageJSON(envPath, paths.ProxyRuntime, &detail.Runtime); err != nil {
			return detail, "", false, err
		}
	}
	return detail, proxyConfigText, proxyConfigExists, nil
}

// readFingerprintDetail 读取指纹摘要。
//
// snapshot.raw 和 backup.raw 不进入响应体；详情只展示状态、评分和可恢复指纹字段。
func readFingerprintDetail(envPath string, paths model.ManifestPaths) (model.BrowserEnvFingerprintDetail, error) {
	var snapshot model.FingerprintSnapshotFile
	if err := readOptionalPackageJSON(envPath, paths.FingerprintSnapshot, &snapshot); err != nil {
		return model.BrowserEnvFingerprintDetail{}, err
	}
	var backup model.FingerprintBackupFile
	if err := readOptionalPackageJSON(envPath, paths.FingerprintBackup, &backup); err != nil {
		return model.BrowserEnvFingerprintDetail{}, err
	}
	runtimeDetail, err := readFingerprintRuntimeConfig(envPath, paths.FingerprintRuntimeConfig)
	if err != nil {
		return model.BrowserEnvFingerprintDetail{}, err
	}

	return model.BrowserEnvFingerprintDetail{
		Snapshot: model.BrowserEnvFingerprintSnapshotDetail{
			OK:        snapshot.OK,
			Source:    snapshot.Source,
			TestedAt:  snapshot.TestedAt,
			TargetURL: snapshot.TargetURL,
			PageURL:   snapshot.PageURL,
			Title:     snapshot.Title,
			Score:     snapshot.Score,
		},
		Backup: model.BrowserEnvFingerprintBackupDetail{
			Available:          backup.Available,
			SavedAt:            backup.SavedAt,
			SourceSnapshotPath: backup.SourceSnapshotPath,
			HasFingerprint:     backup.Fingerprint != nil,
			Fingerprint:        backup.Fingerprint,
		},
		RuntimeConfig: runtimeDetail,
	}, nil
}

// readOptionalPackageJSON 读取可选 JSON 文件。
//
// 指纹和代理运行态文件可能在早期环境包中不存在；缺失不是致命错误，损坏才是错误。
func readOptionalPackageJSON(envPath string, relativePath string, target any) error {
	if strings.TrimSpace(relativePath) == "" {
		return nil
	}
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	if _, err = os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取环境包文件状态失败 %s: %w", relativePath, err)
	}
	return readJSONFile(path, target)
}

// readFingerprintRuntimeConfig 读取可注入容器的指纹配置。
//
// runtime-config.json 第一版可能是空对象；空对象表示尚无可恢复指纹，不应返回 available=true。
func readFingerprintRuntimeConfig(envPath string, relativePath string) (model.BrowserEnvFingerprintRuntimeDetail, error) {
	if strings.TrimSpace(relativePath) == "" {
		return model.BrowserEnvFingerprintRuntimeDetail{}, nil
	}
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return model.BrowserEnvFingerprintRuntimeDetail{}, err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return model.BrowserEnvFingerprintRuntimeDetail{}, nil
		}
		return model.BrowserEnvFingerprintRuntimeDetail{}, fmt.Errorf("读取指纹 runtime-config 失败: %w", err)
	}
	if strings.TrimSpace(string(bytes)) == "" || strings.TrimSpace(string(bytes)) == "{}" {
		return model.BrowserEnvFingerprintRuntimeDetail{Available: false}, nil
	}
	var fingerprint model.RestorableFingerprintConfig
	if err = json.Unmarshal(bytes, &fingerprint); err != nil {
		return model.BrowserEnvFingerprintRuntimeDetail{}, fmt.Errorf("解析指纹 runtime-config 失败: %w", err)
	}
	return model.BrowserEnvFingerprintRuntimeDetail{
		Available:   true,
		Fingerprint: &fingerprint,
	}, nil
}

// buildDetailConsistency 生成详情页轻量一致性检查结果。
//
// 这里不访问 Docker，也不修复文件；它只是帮助前端和开发者判断“数据库索引”和“环境包文件”是否一致。
func buildDetailConsistency(index *model.BrowserEnvIndex, envPath string, manifest model.ManifestFile, profile model.ProfileFile, binding model.BindingFile, proxyConfigText string, proxyConfigExists bool) model.BrowserEnvConsistencyCheck {
	result := model.BrowserEnvConsistencyCheck{
		ManifestMatchesIndex: manifest.EnvID == index.EnvID && manifest.UserID == index.UserID && manifest.RPAType == index.RPAType,
		ProxyConfigExists:    proxyConfigExists,
		BrowserDataExists:    pathExists(filepath.Join(envPath, filepath.FromSlash(manifest.Paths.BrowserData))),
		Errors:               []string{},
	}
	if !result.ManifestMatchesIndex {
		result.Errors = append(result.Errors, "manifest 与数据库索引不一致")
	}
	if profile.EnvID != manifest.EnvID || profile.RPAType != manifest.RPAType {
		result.Errors = append(result.Errors, "profile 与 manifest 不一致")
	}
	if binding.Identity.UserID != manifest.UserID || binding.Identity.RPAType != manifest.RPAType {
		result.Errors = append(result.Errors, "binding 与 manifest 不一致")
	}
	proxyHash := buildTextHash(proxyConfigText)
	identity := buildBindingIdentityFromProfile(manifest.UserID, profile, manifest.Paths, proxyHash)
	identityHash, err := buildJSONHash(identity)
	if err != nil {
		result.Errors = append(result.Errors, "重新计算 identityHash 失败: "+err.Error())
	} else {
		result.IdentityHashMatches = identityHash == binding.IdentityHash
		if !result.IdentityHashMatches {
			result.Errors = append(result.Errors, "identityHash 与 binding 不一致")
		}
	}
	if profile.Proxy.Enabled && !proxyConfigExists {
		result.Errors = append(result.Errors, "代理已启用但配置文件不存在")
	}
	if !result.BrowserDataExists {
		result.Errors = append(result.Errors, "browser-data/profile 目录不存在")
	}
	return result
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func toBindingDetail(binding model.BindingFile) model.BrowserEnvBindingDetail {
	return model.BrowserEnvBindingDetail{
		ID:                binding.ID,
		Version:           binding.Version,
		Locked:            binding.Locked,
		IdentityHash:      binding.IdentityHash,
		ConfigHash:        binding.ConfigHash,
		Identity:          binding.Identity,
		Storage:           binding.Storage,
		SessionState:      binding.SessionState,
		Fingerprint:       binding.Fingerprint,
		RuntimeProtection: binding.RuntimeProtection,
		CreatedAt:         binding.CreatedAt,
		UpdatedAt:         binding.UpdatedAt,
	}
}

func buildDetailFiles(paths model.ManifestPaths) map[string]string {
	return map[string]string{
		"manifest":                 "manifest.json",
		"profile":                  paths.Profile,
		"binding":                  paths.Binding,
		"container":                paths.Container,
		"browserData":              paths.BrowserData,
		"fingerprintSnapshot":      paths.FingerprintSnapshot,
		"fingerprintBackup":        paths.FingerprintBackup,
		"fingerprintRuntimeConfig": paths.FingerprintRuntimeConfig,
		"proxyConfig":              paths.ProxyConfig,
		"proxyRuntime":             paths.ProxyRuntime,
		"logs":                     paths.Logs,
	}
}
