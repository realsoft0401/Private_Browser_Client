package BrowserEnv

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	model "private_browser_client/Models/BrowserEnv"
	packageService "private_browser_client/Service/Package"
)

// GetDetail 返回单个环境包的结构化详情。
//
// 设计来源：
// - 列表接口只给前端展示摘要，不足以支撑单环境排障和后续配置修改；
// - 详情必须围绕 SQLite 索引和环境包文件共同组成，不能只看列表摘要；
// - 代理明文和 fingerprint raw 属于敏感或低价值大字段，不能直接塞进详情响应。
//
// 职责边界：
// - 负责读取 SQLite 索引，再按 envPath 读取 profile/binding/container 等本机文件事实；
// - 负责返回一致性检查和运行态 VNC 摘要；
// - 不做 Docker 实时探测，不修改任何环境包文件。
func (s *Service) GetDetail(envID string, httpBase string, wsBase string) (*model.BrowserEnvDetailResponse, error) {
	index, err := loadBrowserEnvIndexOrFail(strings.TrimSpace(envID))
	if err != nil {
		return nil, err
	}

	attachRunningVNCLinks([]*model.BrowserEnvIndex{index}, httpBase, wsBase)

	if index.Status == model.BrowserEnvStatusBackedUp {
		return buildBackedUpBrowserEnvDetail(index), nil
	}

	detail, err := loadBrowserEnvDetail(index)
	if err != nil {
		return nil, err
	}
	detail.Index = index
	reconcileDetailContainerWithIndex(detail, index)
	if index.VNCURL != "" || index.VNCWSURL != "" || index.WebVNCURL != "" {
		detail.VNC = &model.BrowserEnvDetailVNCConnection{
			VNCURL:    index.VNCURL,
			VNCWSURL:  index.VNCWSURL,
			WebVNCURL: index.WebVNCURL,
		}
	}
	return detail, nil
}

// reconcileDetailContainerWithIndex 用索引里的当前运行态摘要覆盖 detail.container 的易漂移字段。
//
// 设计来源：
// - `container.json` 会保留最近一次真实运行的现场摘要，便于 backup/restore/rebuild-index 追历史；
// - 但详情接口展示“当前事实”时，用户主判断仍然必须以 SQLite 索引为准；
// - stop 后如果继续原样透出旧 `container.json`，就会出现 `index.status=stopped` 但
//   `container.status=running` 的误导性结果。
//
// 职责边界：
// - 这里只修正详情响应中的当前运行态摘要；
// - 不修改 `container.json`、不回写 SQLite；
// - 保留 image/ports/docker/labels 这类环境包静态材料，避免把 detail 降成纯索引重复。
func reconcileDetailContainerWithIndex(detail *model.BrowserEnvDetailResponse, index *model.BrowserEnvIndex) {
	if detail == nil || index == nil {
		return
	}

	detail.Container.EnvID = index.EnvID
	detail.Container.ContainerID = index.ContainerID
	if index.ContainerName != nil {
		detail.Container.ContainerName = *index.ContainerName
	} else {
		detail.Container.ContainerName = ""
	}
	detail.Container.Status = index.ContainerStatus
	detail.Container.UpdatedAt = index.UpdatedAt

	if index.ContainerStatus != model.ContainerStatusRunning {
		detail.Container.StartedAt = index.LastStartedAt
		detail.Container.StoppedAt = index.LastStoppedAt
	}
}

func buildBackedUpBrowserEnvDetail(index *model.BrowserEnvIndex) *model.BrowserEnvDetailResponse {
	return &model.BrowserEnvDetailResponse{
		Index: index,
		Consistency: model.BrowserEnvConsistencyCheck{
			ProfileMatchesIndex: false,
			IdentityHashMatches: false,
			ProxyConfigExists:   false,
			BrowserDataExists:   false,
			Errors: []string{
				"环境包已备份，源环境目录已释放；请先 restore 后再查看完整文件详情",
			},
		},
		Files: map[string]string{},
	}
}

func loadBrowserEnvDetail(index *model.BrowserEnvIndex) (*model.BrowserEnvDetailResponse, error) {
	envPath, profile, err := loadPackageProfileFromIndex(index)
	if err != nil {
		return nil, err
	}

	var binding model.BindingFile
	if err = readPackageJSON(envPath, profile.Paths.Binding, &binding); err != nil {
		return nil, invalidError(fmt.Sprintf("读取 binding 失败: %v", err))
	}

	var container model.ContainerFile
	if err = readPackageJSON(envPath, profile.Paths.Container, &container); err != nil {
		return nil, invalidError(fmt.Sprintf("读取 container 失败: %v", err))
	}

	proxyDetail, proxyConfigText, proxyConfigExists, err := readProxyDetail(envPath, profile, profile.Paths)
	if err != nil {
		return nil, err
	}
	fingerprintDetail, err := readFingerprintDetail(envPath, profile.Paths)
	if err != nil {
		return nil, err
	}

	return &model.BrowserEnvDetailResponse{
		Profile:     profile,
		Binding:     toBindingDetail(binding),
		Container:   container,
		Proxy:       proxyDetail,
		Fingerprint: fingerprintDetail,
		Consistency: buildDetailConsistency(index, envPath, profile, binding, proxyConfigText, proxyConfigExists),
		Files:       buildDetailFiles(profile.Paths),
	}, nil
}

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
		return internalError(fmt.Sprintf("读取环境包文件状态失败 %s: %v", relativePath, err))
	}
	if err = readJSONFile(path, target); err != nil {
		return internalError(fmt.Sprintf("读取环境包文件失败 %s: %v", relativePath, err))
	}
	return nil
}

func readProxyDetail(envPath string, profile model.ProfileFile, paths model.PackagePaths) (model.BrowserEnvProxyDetail, string, bool, error) {
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
	if strings.TrimSpace(configPath) != "" {
		absoluteConfigPath, err := safePackagePath(envPath, configPath)
		if err != nil {
			return detail, "", false, invalidError(err.Error())
		}
		bytes, err := os.ReadFile(absoluteConfigPath)
		if err == nil {
			proxyConfigText = string(bytes)
			proxyConfigExists = true
			detail.ConfigPath = configPath
			detail.ConfigSizeBytes = len(bytes)
			detail.Mode = effectiveClashMode(proxyConfigText, profile.Proxy.Enabled, profile.Proxy.Type)
		} else if !os.IsNotExist(err) {
			return detail, "", false, internalError(fmt.Sprintf("读取代理配置摘要失败: %v", err))
		}
	}

	if paths.ProxyRuntime != "" {
		if err := readOptionalPackageJSON(envPath, paths.ProxyRuntime, &detail.Runtime); err != nil {
			return detail, "", false, err
		}
	}
	return detail, proxyConfigText, proxyConfigExists, nil
}

func readFingerprintDetail(envPath string, paths model.PackagePaths) (model.BrowserEnvFingerprintDetail, error) {
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
			TargetURL: snapshot.TargetURL,
			PageURL:   snapshot.PageURL,
			Title:     snapshot.Title,
			Score:     snapshot.Score,
		},
		Backup: model.BrowserEnvFingerprintBackupDetail{
			Available:          backup.Available,
			SourceSnapshotPath: backup.SourceSnapshotPath,
			HasFingerprint:     backup.Raw != nil,
			Fingerprint:        backup.Raw,
		},
		RuntimeConfig: runtimeDetail,
	}, nil
}

func readFingerprintRuntimeConfig(envPath string, relativePath string) (model.BrowserEnvFingerprintRuntimeDetail, error) {
	if strings.TrimSpace(relativePath) == "" {
		return model.BrowserEnvFingerprintRuntimeDetail{}, nil
	}
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return model.BrowserEnvFingerprintRuntimeDetail{}, invalidError(err.Error())
	}
	if _, err = os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return model.BrowserEnvFingerprintRuntimeDetail{}, nil
		}
		return model.BrowserEnvFingerprintRuntimeDetail{}, internalError(fmt.Sprintf("读取 runtime-config 状态失败: %v", err))
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return model.BrowserEnvFingerprintRuntimeDetail{}, internalError(fmt.Sprintf("读取 runtime-config 失败: %v", err))
	}
	if strings.TrimSpace(string(bytes)) == "" || strings.TrimSpace(string(bytes)) == "{}" {
		return model.BrowserEnvFingerprintRuntimeDetail{}, nil
	}
	return model.BrowserEnvFingerprintRuntimeDetail{
		Available:   true,
		Fingerprint: map[string]any{},
	}, nil
}

func buildDetailConsistency(index *model.BrowserEnvIndex, envPath string, profile model.ProfileFile, binding model.BindingFile, proxyConfigText string, proxyConfigExists bool) model.BrowserEnvConsistencyCheck {
	errorsList := make([]string, 0, 4)
	profileMatchesIndex := profile.EnvID == index.EnvID && profile.UserID == index.UserID && profile.RPAType == index.RPAType
	if !profileMatchesIndex {
		errorsList = append(errorsList, "profile 与 browser_envs 索引不一致")
	}

	expectedIdentityHash, hashErr := buildJSONHash(buildBindingIdentityFromFacts(profile.EnvID, profile.UserID, profile.RPAType))
	identityHashMatches := hashErr == nil && binding.IdentityHash == expectedIdentityHash && profile.IdentityHash == expectedIdentityHash
	if hashErr != nil {
		errorsList = append(errorsList, "identityHash 计算失败")
	} else if !identityHashMatches {
		errorsList = append(errorsList, "profile/binding identityHash 不一致")
	}

	browserDataExists := false
	if path, err := safePackagePath(envPath, profile.Paths.BrowserData); err == nil {
		if stat, statErr := os.Stat(path); statErr == nil && stat.IsDir() {
			browserDataExists = true
		}
	}
	if !browserDataExists {
		errorsList = append(errorsList, "browser-data/profile 不存在")
	}

	if profile.Proxy.Enabled && !proxyConfigExists {
		errorsList = append(errorsList, "proxy/clash.yaml 不存在")
	}
	if profile.Proxy.Enabled && strings.TrimSpace(proxyConfigText) == "" {
		errorsList = append(errorsList, "proxy/clash.yaml 为空")
	}

	return model.BrowserEnvConsistencyCheck{
		ProfileMatchesIndex: profileMatchesIndex,
		IdentityHashMatches: identityHashMatches,
		ProxyConfigExists:   proxyConfigExists,
		BrowserDataExists:   browserDataExists,
		Errors:              errorsList,
	}
}

func toBindingDetail(binding model.BindingFile) model.BrowserEnvBindingDetail {
	return model.BrowserEnvBindingDetail{
		ID:                binding.ID,
		Version:           binding.Version,
		Locked:            binding.Locked,
		IdentityHash:      binding.IdentityHash,
		Identity:          binding.Identity,
		Storage:           binding.Storage,
		SessionState:      binding.SessionState,
		Fingerprint:       binding.Fingerprint,
		RuntimeProtection: binding.RuntimeProtection,
		CreatedAt:         binding.CreatedAt,
		UpdatedAt:         binding.UpdatedAt,
	}
}

func buildDetailFiles(paths model.PackagePaths) map[string]string {
	return map[string]string{
		"profile":                  paths.Profile,
		"binding":                  paths.Binding,
		"container":                paths.Container,
		"proxyConfig":              paths.ProxyConfig,
		"proxyRuntime":             paths.ProxyRuntime,
		"fingerprintSnapshot":      paths.FingerprintSnapshot,
		"fingerprintBackup":        paths.FingerprintBackup,
		"fingerprintRuntimeConfig": paths.FingerprintRuntimeConfig,
		"browserData":              paths.BrowserData,
	}
}

func effectiveClashMode(config string, enabled bool, proxyType string) string {
	if !enabled || strings.TrimSpace(proxyType) == "" {
		return ""
	}
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "mode:") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "mode:"))
		}
	}
	return ""
}

// attachRunningVNCLinks 给运行中的环境包列表项补充 slot 视角连接地址。
//
// 新架构里 WebVNC 和 VNC/CDP 入口都围绕 slot，而不是 env 直接暴露；
// 因此这里会优先读取 package 当前运行视图里的 currentSlotId，再拼接 slot 视角地址。
func attachRunningVNCLinks(items []*model.BrowserEnvIndex, httpBase string, wsBase string) {
	httpBase = strings.TrimRight(strings.TrimSpace(httpBase), "/")
	wsBase = strings.TrimRight(strings.TrimSpace(wsBase), "/")
	pkgSvc := packageService.NewService()

	for _, item := range items {
		if item == nil || item.Status != model.BrowserEnvStatusRunning || item.VNCPort <= 0 {
			continue
		}
		view, err := pkgSvc.GetByPackageID(item.EnvID)
		if err != nil || view == nil || view.CurrentSlotID == nil || strings.TrimSpace(*view.CurrentSlotID) == "" {
			continue
		}
		slotID := strings.TrimSpace(*view.CurrentSlotID)
		item.VNCURL = publishedVNCURLForClient(httpBase, item.VNCPort)
		if wsBase != "" {
			item.VNCWSURL = fmt.Sprintf("%s/api/v1/edge/slots/%s/vnc/ws", wsBase, url.PathEscape(slotID))
		}
		if httpBase != "" {
			item.WebVNCURL = fmt.Sprintf("%s/web-vnc.html?slot=%s", httpBase, url.QueryEscape(slotID))
		}
	}
}

func publishedVNCURLForClient(httpBase string, port int) string {
	if strings.TrimSpace(httpBase) == "" || port <= 0 {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(httpBase))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return fmt.Sprintf("vnc://%s:%d", parsed.Hostname(), port)
}
