package BrowserEnv

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	TaskService "private_browser_client/Service/Task"
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
	Enabled      bool
	Type         string
	Config       string
	Image        string
	ProxyChanged bool
	ImageChanged bool
	Changed      bool
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
// - running 环境会把 forceRecreate 放到后台队列，避免 rule/CDP timezone 探测拖断 HTTP 请求；
// - 异步策略绑定的是 running 环境的重建任务，不是 rule 模式本身；rule/global/direct 都会后台重建，只是后续探测入口不同；
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
		result.RestartRequired = false
		result.RestartQueued = true
		task := enqueueProxyRecreate(envID, "browser_env_proxy_update")
		if task != nil {
			result.TaskID = task.ID
		}
	}
	return result, nil
}

// UpdateBrowserEnvProxyMode 修改环境包 Clash 代理模式。
//
// 设计来源：
// - 用户确认规则/全局模式应该做成接口，而不是塞进 run 参数；
// - mode 是 proxy/clash.yaml 的稳定配置事实，必须随环境包备份、导出和导入一起迁移；
// - running 环境切换 mode 后仍然要自动重建容器，并重新走 timezone 探测，避免出口与浏览器 TZ 不一致。
//
// 职责边界：
// - 只修改 YAML 顶层 mode 字段，允许值为 rule/global/direct；
// - 不修改代理节点、规则列表、proxy-groups 或其它 Clash 字段；
// - running 环境把重建放入后台队列，避免外部 provider/CDP 耗时导致接口 socket hang up；
// - 不要把它理解成“只有 rule 异步”，异步的是 running 环境重建，mode 只决定后台 timezone probe 走 CDP 还是 curl；
// - 不返回代理正文，不删除 browser-data/profile。
func (s *Service) UpdateBrowserEnvProxyMode(envID string, param *model.UpdateBrowserEnvProxyModeRequest) (*model.UpdateBrowserEnvProxyResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}
	if param == nil {
		return nil, invalidError("请求参数不能为空")
	}
	mode, err := normalizeClashMode(param.Mode)
	if err != nil {
		return nil, err
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
	if !pkg.Profile.Proxy.Enabled {
		return nil, conflictError("代理未启用，不能切换代理模式")
	}
	updatedConfig, changed, err := replaceClashMode(pkg.ProxyConfig, mode)
	if err != nil {
		return nil, err
	}
	if !changed {
		return buildProxyUpdateResponse(pkg.Index.EnvID, pkg.Index.Status, pkg.Binding, pkg.Profile, pkg.ProxyConfig, false, false, pkg.Index.UpdatedAt), nil
	}
	normalized := &normalizedProxyUpdate{
		Enabled:      true,
		Type:         firstNonEmpty(pkg.Profile.Proxy.Type, "clash-verge"),
		Config:       updatedConfig,
		ProxyChanged: true,
		Changed:      true,
	}
	result, err := finalizeProxyUpdate(pkg, normalized)
	if err != nil {
		return nil, err
	}
	if wasRunning {
		result.RestartRequired = false
		result.RestartQueued = true
		task := enqueueProxyRecreate(envID, "browser_env_proxy_mode_update")
		if task != nil {
			result.TaskID = task.ID
		}
	}
	return result, nil
}

// enqueueProxyRecreate 异步重建运行中的浏览器容器。
//
// 设计来源：
// - 用户实测 rule 模式下 PATCH proxy 会因为 CDP/timezone 耗时过长出现 socket hang up；
// - 配置落盘和 identityHash 更新必须在当前请求内完成，但 Docker 重建和 provider 探测可以后台串行执行；
// - runBrowserEnvLocked 内部仍会持有全局 runEnvMu，因此多个后台重建会排队，不会并发抢同一个容器。
//
// 职责边界：
// - 只负责触发 forceRecreate，不吞掉 run 内部的状态落库；
// - 不向前端推送进度，前端可通过详情/列表查看 container_status、last_error 和 proxy-runtime。
func enqueueProxyRecreate(envID string, taskType string) *TaskService.EdgeTask {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil
	}
	task := TaskService.Create(taskType, "browser_env", envID, "代理配置已写入，后台重建任务已创建")
	go func(id string, taskID string) {
		TaskService.MarkRunning(taskID, "container_recreate", "开始后台重建浏览器容器", map[string]any{"envId": id})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go TaskService.RunHeartbeat(ctx, taskID, "container_recreate", "浏览器容器重建仍在执行")
		result, err := NewService().RunBrowserEnv(id, &model.RunBrowserEnvRequest{ForceRecreate: true})
		if err != nil {
			TaskService.Failed(taskID, "container_recreate", err.Error())
			return
		}
		TaskService.Done(taskID, "container_recreate", "浏览器容器重建完成", result)
	}(envID, task.ID)
	return task
}

// ensureProxyUpdateStatus 判断当前状态是否允许修改代理配置。
//
// running 允许进入配置修改，但必须由 UpdateBrowserEnvProxy 自己完成 forceRecreate；
// deleted/archived/backed_up 不允许修改，是为了避免回收站或只有备份包的资产重新变成半活跃环境。
func ensureProxyUpdateStatus(status string) error {
	switch status {
	case model.BrowserEnvStatusCreated, model.BrowserEnvStatusStopped, model.BrowserEnvStatusError, model.BrowserEnvStatusRunning:
		return nil
	case model.BrowserEnvStatusDeleted:
		return conflictError("环境包已删除，不能修改配置")
	case model.BrowserEnvStatusBackedUp:
		return conflictError("环境包当前只有备份包，请先 restore 后再修改配置")
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
// image 是 profile.runtime.image 的局部更新入口，和代理配置共用一次重建队列，但不参与 identityHash。
// mode 属于 Clash YAML 顶层字段：启用代理时可单独修改，也可和 configBase64 一起覆盖；
// 禁用代理时会清空代理类型和配置，忽略 mode，避免“禁用动作”被空配置校验误伤。
// 只有实际业务值发生变化时，才标记 Changed=true，避免无意义修改也要求重启容器。
func normalizeProxyUpdate(pkg *proxyConfigPackage, param *model.UpdateBrowserEnvProxyRequest) (*normalizedProxyUpdate, error) {
	if param.Enabled == nil && param.Type == nil && param.Image == nil && param.Mode == nil && param.ConfigBase64 == nil {
		return nil, invalidError("至少需要传 enabled/type/image/mode/configBase64 中的一个字段")
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
	image := strings.TrimSpace(pkg.Profile.Runtime.Image)
	if param.Image != nil {
		image = strings.TrimSpace(*param.Image)
		if image == "" {
			return nil, invalidError("image 不能为空")
		}
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
		if param.Mode != nil {
			mode, err := normalizeClashMode(*param.Mode)
			if err != nil {
				return nil, err
			}
			updatedConfig, _, err := replaceClashMode(config, mode)
			if err != nil {
				return nil, err
			}
			config = updatedConfig
		}
	} else {
		proxyType = ""
		config = ""
	}

	proxyChanged := enabled != pkg.Profile.Proxy.Enabled ||
		proxyType != pkg.Profile.Proxy.Type ||
		buildTextHash(config) != buildTextHash(pkg.ProxyConfig)
	imageChanged := image != strings.TrimSpace(pkg.Profile.Runtime.Image)
	return &normalizedProxyUpdate{
		Enabled:      enabled,
		Type:         proxyType,
		Config:       config,
		Image:        image,
		ProxyChanged: proxyChanged,
		ImageChanged: imageChanged,
		Changed:      proxyChanged || imageChanged,
	}, nil
}

func normalizeClashMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "rule", "global", "direct":
		return mode, nil
	default:
		return "", invalidError("mode 仅支持 rule/global/direct")
	}
}

// replaceClashMode 只替换 Clash YAML 顶层 mode 字段。
//
// 这里不用字符串全局替换，避免误伤 rules、proxy-groups 或注释里的 mode；
// 如果原配置没有 mode，则插入到文件开头，保持第一眼可见。
func replaceClashMode(config string, mode string) (string, bool, error) {
	if strings.TrimSpace(config) == "" {
		return "", false, invalidError("代理配置为空，不能切换代理模式")
	}
	re := regexp.MustCompile(`(?m)^(\s*)mode\s*:\s*([A-Za-z_-]+)\s*$`)
	match := re.FindStringSubmatch(config)
	if len(match) == 3 {
		current := strings.ToLower(strings.TrimSpace(match[2]))
		if current == mode {
			return config, false, nil
		}
		updated := re.ReplaceAllString(config, "${1}mode: "+mode)
		return updated, true, nil
	}
	prefix := "mode: " + mode + "\n"
	return prefix + strings.TrimLeft(config, "\n"), true, nil
}

func extractClashMode(config string) string {
	re := regexp.MustCompile(`(?m)^\s*mode\s*:\s*([A-Za-z_-]+)\s*$`)
	match := re.FindStringSubmatch(config)
	if len(match) == 2 {
		return strings.ToLower(strings.TrimSpace(match[1]))
	}
	return ""
}

// effectiveClashMode 返回详情和响应里应该展示的代理模式。
//
// 设计来源：
// - 用户发现详情接口看起来没有代理 mode 状态；
// - 一些历史/导入配置可能没有显式写 `mode:`，但当前 run/timezone 逻辑已经把缺省模式按 rule 处理；
// - 因此展示层不能返回空字符串误导前端，应该把“有效模式”明确暴露为 rule。
//
// 职责边界：
// - 只用于响应展示，不改写 proxy/clash.yaml；
// - 禁用代理或非 clash-verge 代理不强行补 rule，避免让前端误判没有 Clash 配置的环境包。
func effectiveClashMode(config string, enabled bool, proxyType string) string {
	mode := extractClashMode(config)
	if mode != "" {
		return mode
	}
	if enabled && strings.EqualFold(strings.TrimSpace(proxyType), "clash-verge") && strings.TrimSpace(config) != "" {
		return "rule"
	}
	return ""
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

// finalizeProxyUpdate 写入代理配置和运行镜像修改。
//
// 代理和镜像都不参与 identityHash。代理变更只会递增 binding.version，
// 并把 runtimeProtection/proxyRuntime 重置为 pending，等待下一次 run 重新做网络指纹确认。
func finalizeProxyUpdate(pkg *proxyConfigPackage, update *normalizedProxyUpdate) (*model.UpdateBrowserEnvProxyResponse, error) {
	now := time.Now().Unix()
	pkg.Profile.Proxy.Enabled = update.Enabled
	pkg.Profile.Proxy.Type = update.Type
	if update.ImageChanged {
		pkg.Profile.Runtime.Image = update.Image
	}
	if strings.TrimSpace(pkg.Profile.Proxy.ConfigPath) == "" {
		pkg.Profile.Proxy.ConfigPath = pkg.Manifest.Paths.ProxyConfig
	}
	pkg.Profile.Metadata.UpdatedAt = now

	if update.ProxyChanged {
		pkg.Binding.Version++
		pkg.Binding.RuntimeProtection.TimezoneStatus = "pending"
		pkg.Binding.RuntimeProtection.RiskStatus = "pending"
		pkg.Binding.RuntimeProtection.AvailabilityStatus = "pending"
		pkg.Binding.RuntimeProtection.LastError = ""
		pkg.Binding.UpdatedAt = now
	}
	pkg.Manifest.UpdatedAt = now

	if err := writeJSONFile(filepath.Join(pkg.AbsoluteEnvPath, "manifest.json"), pkg.Manifest); err != nil {
		return nil, internalError(err.Error())
	}
	if err := writePackageJSON(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.Profile, pkg.Profile); err != nil {
		return nil, internalError(err.Error())
	}
	if update.ProxyChanged {
		if err := writePackageJSON(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.Binding, pkg.Binding); err != nil {
			return nil, internalError(err.Error())
		}
		if err := writePackageText(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.ProxyConfig, update.Config); err != nil {
			return nil, internalError(err.Error())
		}
		if err := writeTimezoneProbePending(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.ProxyRuntime); err != nil {
			return nil, internalError(err.Error())
		}
	}

	nextStatus := pkg.Index.Status
	if nextStatus == model.BrowserEnvStatusError {
		nextStatus = model.BrowserEnvStatusStopped
	}
	if err := browserEnvDao.NewConfigModelHandler().UpdateBrowserEnvConfig(context.Background(), &model.BrowserEnvConfigUpdate{
		EnvID:     pkg.Index.EnvID,
		Status:    nextStatus,
		UpdatedAt: now,
	}); err != nil {
		return nil, internalError(err.Error())
	}
	return buildProxyUpdateResponse(pkg.Index.EnvID, nextStatus, pkg.Binding, pkg.Profile, update.Config, true, true, now), nil
}

// writeTimezoneProbePending 标记代理变化后需要在下一次 run 中重新确认容器内 timezone。
//
// 非 running 环境修改代理时不会隐式启动容器，因此这里仅写入 pending 状态；
// running 环境随后会 forceRecreate 并由 run 流程覆盖为 verified 或 failed。
func writeTimezoneProbePending(envPath string, relativePath string) error {
	source := "container-probe"
	runtime := model.ProxyRuntimeFile{
		Source: &source,
		Status: "pending",
		Drift:  false,
	}
	return writePackageJSON(envPath, relativePath, runtime)
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
		Image:  profile.Runtime.Image,
		Proxy: model.BrowserEnvProxyDetail{
			Enabled:         profile.Proxy.Enabled,
			Type:            profile.Proxy.Type,
			Mode:            effectiveClashMode(config, profile.Proxy.Enabled, profile.Proxy.Type),
			ConfigPath:      profile.Proxy.ConfigPath,
			ConfigSizeBytes: len([]byte(config)),
		},
		BindingVersion:  binding.Version,
		IdentityHash:    binding.IdentityHash,
		Changed:         changed,
		RestartRequired: restartRequired,
		UpdatedAt:       updatedAt,
	}
}
