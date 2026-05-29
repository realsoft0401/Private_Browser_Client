package BrowserEnv

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Settings"
)

type proxyConfigPackage struct {
	Index           *model.BrowserEnvIndex
	Manifest        model.ManifestFile
	Profile         model.ProfileFile
	Binding         model.BindingFile
	ProxyConfig     string
	AbsoluteEnvPath string
}

type normalizedProxyUpdate struct {
	Enabled bool
	Type    string
	Config  string
	Changed bool
}

// UpdateBrowserEnvProxy 修改环境包代理配置。
//
// 设计来源：
// - 用户明确要求“只要改的东西，就需要重新启动容器”；
// - 代理配置会被 run 编码进容器环境变量，同时参与 binding.identityHash；
// - 用户进一步确认“启动对用户来说是无感知的”，因此 running 状态下真实变更会由本接口自动重建容器。
//
// 职责边界：
// - 负责修改 profile.proxy、proxy/clash.yaml、binding.identityHash 和 manifest.updatedAt；
// - 负责同步 browser_envs.updated_at，并在 error 状态下把环境包收敛回 stopped 以便重新 run；
// - running 环境会自动 forceRecreate 让新配置生效；
// - 不删除 browser-data/profile，不返回代理正文。
func (s *Service) UpdateBrowserEnvProxy(envID string, param *model.UpdateBrowserEnvProxyRequest) (*model.UpdateBrowserEnvProxyResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}
	if param == nil {
		return nil, invalidError("请求参数不能为空")
	}

	runEnvMu.Lock()
	defer runEnvMu.Unlock()

	index, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}
	if err = ensureProxyUpdateStatus(index.Status); err != nil {
		return nil, err
	}
	wasRunning := index.Status == model.BrowserEnvStatusRunning

	pkg, err := loadProxyConfigPackage(index)
	if err != nil {
		return nil, internalError(err.Error())
	}
	normalized, err := normalizeProxyUpdate(pkg, param)
	if err != nil {
		return nil, err
	}
	if !normalized.Changed {
		return buildProxyUpdateResponse(pkg.Index.EnvID, pkg.Index.Status, pkg.Binding, pkg.Profile, pkg.ProxyConfig, false, false, pkg.Index.UpdatedAt), nil
	}
	result, err := finalizeProxyUpdate(pkg, normalized)
	if err != nil {
		return nil, err
	}
	if wasRunning {
		runResult, err := runBrowserEnvLocked(envID, &model.RunBrowserEnvRequest{ForceRecreate: true})
		if err != nil {
			return nil, err
		}
		result.Status = runResult.Status
		result.RestartRequired = false
		result.Restarted = true
		result.Run = runResult
	}
	return result, nil
}

// ensureProxyUpdateStatus 判断当前状态是否允许修改代理配置。
//
// running 允许进入配置修改，但必须由 UpdateBrowserEnvProxy 自己完成 forceRecreate；
// deleted/archived 不允许修改，是为了避免回收站或归档数据重新变成活跃环境。
func ensureProxyUpdateStatus(status string) error {
	switch status {
	case model.BrowserEnvStatusCreated, model.BrowserEnvStatusStopped, model.BrowserEnvStatusError, model.BrowserEnvStatusRunning:
		return nil
	case model.BrowserEnvStatusDeleted:
		return conflictError("环境包已删除，不能修改配置")
	case model.BrowserEnvStatusArchived:
		return conflictError("环境包已归档，不能修改配置")
	default:
		return conflictError("当前状态不能修改配置")
	}
}

// loadProxyConfigPackage 读取代理修改所需的环境包文件。
//
// 它只读取 manifest/profile/binding/proxyConfig，不读取 fingerprint raw，也不扫描 browser-data；
// 代理修改必须围绕环境包内相对路径执行，避免导入包里出现路径逃逸。
func loadProxyConfigPackage(index *model.BrowserEnvIndex) (*proxyConfigPackage, error) {
	if index == nil {
		return nil, fmt.Errorf("环境包索引不能为空")
	}
	if Settings.Conf.ProjectRoot == "" {
		return nil, fmt.Errorf("project root 不能为空")
	}
	absoluteEnvPath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(index.EnvPath))
	var manifest model.ManifestFile
	if err := readJSONFile(filepath.Join(absoluteEnvPath, "manifest.json"), &manifest); err != nil {
		return nil, err
	}
	if manifest.EnvID != index.EnvID {
		return nil, fmt.Errorf("manifest.envId 与数据库索引不一致")
	}
	var profile model.ProfileFile
	if err := readPackageJSON(absoluteEnvPath, manifest.Paths.Profile, &profile); err != nil {
		return nil, err
	}
	var binding model.BindingFile
	if err := readPackageJSON(absoluteEnvPath, manifest.Paths.Binding, &binding); err != nil {
		return nil, err
	}
	proxyConfig := ""
	if strings.TrimSpace(manifest.Paths.ProxyConfig) != "" {
		proxyPath, err := safePackagePath(absoluteEnvPath, manifest.Paths.ProxyConfig)
		if err != nil {
			return nil, err
		}
		bytes, err := os.ReadFile(proxyPath)
		if err == nil {
			proxyConfig = string(bytes)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("读取代理配置失败: %w", err)
		}
	}
	return &proxyConfigPackage{
		Index:           index,
		Manifest:        manifest,
		Profile:         profile,
		Binding:         binding,
		ProxyConfig:     proxyConfig,
		AbsoluteEnvPath: absoluteEnvPath,
	}, nil
}

// normalizeProxyUpdate 合并 PATCH 请求和现有代理配置。
//
// PATCH 允许前端只传被修改字段；但最终落盘必须是完整 profile.proxy + proxy/clash.yaml。
// 只有实际业务值发生变化时，才标记 Changed=true，避免无意义修改也要求重启容器。
func normalizeProxyUpdate(pkg *proxyConfigPackage, param *model.UpdateBrowserEnvProxyRequest) (*normalizedProxyUpdate, error) {
	if param.Enabled == nil && param.Type == nil && param.ConfigBase64 == nil {
		return nil, invalidError("至少需要传 enabled/type/configBase64 中的一个字段")
	}
	enabled := pkg.Profile.Proxy.Enabled
	if param.Enabled != nil {
		enabled = *param.Enabled
	}
	proxyType := pkg.Profile.Proxy.Type
	if param.Type != nil {
		proxyType = strings.TrimSpace(*param.Type)
	}
	config := pkg.ProxyConfig
	if param.ConfigBase64 != nil {
		decoded, err := decodeProxyConfigBase64(*param.ConfigBase64)
		if err != nil {
			return nil, err
		}
		config = decoded
	}

	if enabled {
		if proxyType == "" {
			proxyType = "clash-verge"
		}
		if proxyType != "clash-verge" {
			return nil, invalidError("proxy.type 第一版仅支持 clash-verge")
		}
		if strings.TrimSpace(config) == "" {
			return nil, invalidError("proxy.enabled=true 时 proxy.configBase64 不能为空")
		}
	} else {
		proxyType = ""
		config = ""
	}

	changed := enabled != pkg.Profile.Proxy.Enabled ||
		proxyType != pkg.Profile.Proxy.Type ||
		buildTextHash(config) != buildTextHash(pkg.ProxyConfig)
	return &normalizedProxyUpdate{
		Enabled: enabled,
		Type:    proxyType,
		Config:  config,
		Changed: changed,
	}, nil
}

// decodeProxyConfigBase64 解码 PATCH 请求里的代理配置。
//
// 设计来源：
// - 代理 YAML 很长，里面包含换行、双引号、通配符和账号密码，直接放 JSON 字符串容易被截断或转义错；
// - 用户确认后，正式传参方式改为 configBase64；
// - 这里会容忍前端把长 Base64 自动折行，但不会吞掉解码错误，避免坏配置继续落盘。
func decodeProxyConfigBase64(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	normalized = strings.ReplaceAll(normalized, "\n", "")
	normalized = strings.ReplaceAll(normalized, "\r", "")
	normalized = strings.ReplaceAll(normalized, "\t", "")
	normalized = strings.ReplaceAll(normalized, " ", "")
	if normalized == "" {
		return "", invalidError("proxy.configBase64 不能为空")
	}
	bytes, err := base64.StdEncoding.DecodeString(normalized)
	if err != nil {
		if rawBytes, rawErr := base64.RawStdEncoding.DecodeString(normalized); rawErr == nil {
			bytes = rawBytes
		} else {
			return "", invalidError("proxy.configBase64 不是有效 Base64")
		}
	}
	return string(bytes), nil
}

// finalizeProxyUpdate 写入代理配置修改并同步 identityHash。
//
// 代理 hash 参与账号环境身份，修改代理后必须递增 binding.version 并重算 identityHash；
// 但 browser-data/profile 仍然保留，是否继续使用该登录态由业务策略和后续风控检查决定。
func finalizeProxyUpdate(pkg *proxyConfigPackage, update *normalizedProxyUpdate) (*model.UpdateBrowserEnvProxyResponse, error) {
	now := time.Now().Unix()
	pkg.Profile.Proxy.Enabled = update.Enabled
	pkg.Profile.Proxy.Type = update.Type
	if strings.TrimSpace(pkg.Profile.Proxy.ConfigPath) == "" {
		pkg.Profile.Proxy.ConfigPath = pkg.Manifest.Paths.ProxyConfig
	}
	pkg.Profile.Metadata.UpdatedAt = now

	proxyHash := buildTextHash(update.Config)
	identity := buildBindingIdentityFromProfile(pkg.Manifest.UserID, pkg.Profile, pkg.Manifest.Paths, proxyHash)
	identityHash, err := buildJSONHash(identity)
	if err != nil {
		return nil, internalError(fmt.Sprintf("计算 identityHash 失败: %v", err))
	}
	pkg.Binding.Version++
	pkg.Binding.Identity = identity
	pkg.Binding.IdentityHash = identityHash
	pkg.Binding.ConfigHash = identityHash
	pkg.Binding.UpdatedAt = now
	pkg.Manifest.UpdatedAt = now

	if err = writeJSONFile(filepath.Join(pkg.AbsoluteEnvPath, "manifest.json"), pkg.Manifest); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.Profile, pkg.Profile); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.Binding, pkg.Binding); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageText(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.ProxyConfig, update.Config); err != nil {
		return nil, internalError(err.Error())
	}

	nextStatus := pkg.Index.Status
	if nextStatus == model.BrowserEnvStatusError {
		nextStatus = model.BrowserEnvStatusStopped
	}
	if err = browserEnvDao.NewConfigModelHandler().UpdateBrowserEnvConfig(context.Background(), &model.BrowserEnvConfigUpdate{
		EnvID:     pkg.Index.EnvID,
		Status:    nextStatus,
		UpdatedAt: now,
	}); err != nil {
		return nil, internalError(err.Error())
	}
	return buildProxyUpdateResponse(pkg.Index.EnvID, nextStatus, pkg.Binding, pkg.Profile, update.Config, true, true, now), nil
}

// writePackageJSON 在环境包目录内安全写入 JSON。
//
// 路径必须来自 manifest 的相对路径，不能让接口参数直接控制写入位置。
func writePackageJSON(envPath string, relativePath string, value any) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	return writeJSONFile(path, value)
}

// writePackageText 在环境包目录内安全写入文本。
//
// 当前主要用于 proxy/clash.yaml；代理正文可能包含多行 YAML，必须原样写入文件。
func writePackageText(envPath string, relativePath string, value string) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	return writeTextFile(path, value)
}

func buildProxyUpdateResponse(envID string, status string, binding model.BindingFile, profile model.ProfileFile, config string, changed bool, restartRequired bool, updatedAt int64) *model.UpdateBrowserEnvProxyResponse {
	return &model.UpdateBrowserEnvProxyResponse{
		EnvID:  envID,
		Status: status,
		Proxy: model.BrowserEnvProxyDetail{
			Enabled:         profile.Proxy.Enabled,
			Type:            profile.Proxy.Type,
			ConfigPath:      profile.Proxy.ConfigPath,
			ConfigHash:      buildTextHash(config),
			ConfigSizeBytes: len([]byte(config)),
		},
		BindingVersion:  binding.Version,
		IdentityHash:    binding.IdentityHash,
		Changed:         changed,
		RestartRequired: restartRequired,
		UpdatedAt:       updatedAt,
	}
}
