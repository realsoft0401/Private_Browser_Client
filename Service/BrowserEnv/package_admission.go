package BrowserEnv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	model "private_browser_client/Models/BrowserEnv"
	edgeService "private_browser_client/Service/Edge"
)

const (
	rebuildCandidateCreatedAtomic = "created_atomic"
	rebuildCandidateInvalid       = "invalid"
)

// atomicPackage 是通过 profile.json 入口校验后的环境包视图。
//
// 设计来源：
// - 用户确认“一个环境包就是一个具有原子性的独立个体”，profile.json 是唯一主配置；
// - import、restore、rebuild-index 都属于从文件恢复系统状态，不能各自猜测哪些文件可缺省；
// - 因此这里集中检查原子材料、身份摘要、路径安全和配置格式。
//
// 职责边界：
// - 只读取环境包内的结构化配置和必要文件，不读取 browser-data/profile 内部登录态内容；
// - 不修复、不补默认值、不分配端口、不写 SQLite；
// - 缺少原子材料、JSON/YAML 非法、身份不一致都返回 invalid，调用方不能继续落库。
type atomicPackage struct {
	EnvPath          string
	Profile          model.ProfileFile
	Binding          model.BindingFile
	Container        model.ContainerFile
	HasContainerFile bool
}

func loadAndValidateAtomicPackage(envPath string) (*atomicPackage, error) {
	var profile model.ProfileFile
	if err := readJSONFile(filepath.Join(envPath, "profile.json"), &profile); err != nil {
		return nil, invalidError(fmt.Sprintf("读取 profile 失败: %v", err))
	}
	if err := validateProfileIdentityAndPaths(envPath, profile); err != nil {
		return nil, err
	}

	var binding model.BindingFile
	if err := readPackageJSON(envPath, profile.Paths.Binding, &binding); err != nil {
		return nil, invalidError(fmt.Sprintf("读取 binding 失败: %v", err))
	}
	if err := validateBindingMatchesProfile(profile, binding); err != nil {
		return nil, err
	}

	container := model.ContainerFile{}
	hasContainer := false
	if strings.TrimSpace(profile.Paths.Container) != "" {
		containerPath, err := safePackagePath(envPath, profile.Paths.Container)
		if err != nil {
			return nil, invalidError(err.Error())
		}
		if _, err = os.Stat(containerPath); err == nil {
			hasContainer = true
			if err = readJSONFile(containerPath, &container); err != nil {
				return nil, invalidError(fmt.Sprintf("读取 container 失败: %v", err))
			}
			if container.EnvID != "" && container.EnvID != profile.EnvID {
				return nil, invalidError("container.envId 与 profile.envId 不一致")
			}
		} else if !os.IsNotExist(err) {
			return nil, invalidError(fmt.Sprintf("读取 container 状态失败: %v", err))
		}
	}

	if err := requireAtomicPackageMaterials(envPath, profile); err != nil {
		return nil, err
	}
	return &atomicPackage{
		EnvPath:          envPath,
		Profile:          profile,
		Binding:          binding,
		Container:        container,
		HasContainerFile: hasContainer,
	}, nil
}

func validateProfileIdentityAndPaths(envPath string, profile model.ProfileFile) error {
	if profile.SchemaVersion != model.SchemaVersion {
		return invalidError("profile.schemaVersion 不支持")
	}
	if strings.TrimSpace(profile.EnvID) == "" || strings.TrimSpace(profile.UserID) == "" || strings.TrimSpace(profile.RPAType) == "" {
		return invalidError("profile.envId/userId/rpaType 不能为空")
	}
	if _, ok := model.SupportedRPATypes[profile.RPAType]; !ok {
		return invalidError("profile.rpaType 不支持")
	}
	if filepath.Base(envPath) != profile.EnvID {
		return invalidError("环境包目录名必须等于 profile.envId")
	}
	if strings.TrimSpace(profile.Paths.Profile) != "profile.json" {
		return invalidError("profile.paths.profile 必须等于 profile.json")
	}
	if err := validateProfilePackagePaths(profile.Paths); err != nil {
		return invalidError(err.Error())
	}
	identity := buildBindingIdentityFromFacts(profile.EnvID, profile.UserID, profile.RPAType)
	identityHash, err := buildJSONHash(identity)
	if err != nil {
		return internalError(fmt.Sprintf("计算 identityHash 失败: %v", err))
	}
	if strings.TrimSpace(profile.IdentityHash) == "" {
		return invalidError("profile.identityHash 不能为空")
	}
	if profile.IdentityHash != identityHash {
		return invalidError("profile.identityHash 与 envId/userId/rpaType 不一致")
	}
	return nil
}

func validateBindingMatchesProfile(profile model.ProfileFile, binding model.BindingFile) error {
	if binding.Identity.EnvID != profile.EnvID ||
		binding.Identity.UserID != profile.UserID ||
		binding.Identity.RPAType != profile.RPAType {
		return invalidError("binding.identity 与 profile 不一致")
	}
	if binding.IdentityHash != profile.IdentityHash {
		return invalidError("binding.identityHash 与 profile.identityHash 不一致")
	}
	if strings.TrimSpace(binding.Storage.HostUserDataDir) != profile.Paths.BrowserData {
		return invalidError("binding.storage.hostUserDataDir 与 profile.paths.browserData 不一致")
	}
	if strings.TrimSpace(binding.Storage.ContainerUserDataDir) == "" {
		return invalidError("binding.storage.containerUserDataDir 不能为空")
	}
	return nil
}

func requireAtomicPackageMaterials(envPath string, profile model.ProfileFile) error {
	requiredFiles := []string{
		profile.Paths.Profile,
		profile.Paths.Binding,
		profile.Paths.FingerprintSnapshot,
		profile.Paths.FingerprintBackup,
		profile.Paths.FingerprintRuntimeConfig,
		profile.Paths.ProxyRuntime,
	}
	for _, relativePath := range requiredFiles {
		if err := requirePackageFile(envPath, relativePath); err != nil {
			return invalidError(err.Error())
		}
	}
	requiredDirs := []string{
		"profile.browserData=" + profile.Paths.BrowserData,
		"proxy",
		"fingerprint",
	}
	for _, item := range requiredDirs {
		name := item
		path := item
		if strings.Contains(item, "=") {
			parts := strings.SplitN(item, "=", 2)
			name = parts[0]
			path = parts[1]
		}
		if err := requirePackageDir(envPath, path); err != nil {
			return invalidError(strings.Replace(err.Error(), path, name, 1))
		}
	}
	if err := validatePackageJSONFile(envPath, profile.Paths.FingerprintSnapshot, &model.FingerprintSnapshotFile{}); err != nil {
		return err
	}
	if err := validatePackageJSONFile(envPath, profile.Paths.FingerprintBackup, &model.FingerprintBackupFile{}); err != nil {
		return err
	}
	if err := validatePackageJSONFile(envPath, profile.Paths.ProxyRuntime, &model.ProxyRuntimeFile{}); err != nil {
		return err
	}
	if err := validateProxyMaterialsForAdmission(envPath, profile); err != nil {
		return err
	}
	return nil
}

// validateProxyMaterialsForAdmission 统一处理环境包准入时的代理材料校验。
//
// 设计来源：
//   - 用户已经确认“proxy.enabled=false”是新项目里的正式状态，不需要为了旧兼容强制保留 clash.yaml；
//   - 之前准入层把 proxy/clash.yaml 当成所有环境包的必备原子材料，导致 backup/import/revalidate/rebuild-index
//     在关闭代理时误报“环境包损坏”，这和详情接口、业务语义都不一致；
//   - 因此现在收口为：proxyRuntime 和 proxy 目录始终必需，clash.yaml 只在代理启用时才是硬性材料。
//
// 职责边界：
// - 这里只判断“当前生命周期动作是否还能信任这个环境包”；
// - 不负责修复代理配置，也不因为代理关闭去推导默认 clash 配置；
// - 如果代理关闭，即使磁盘上没有 clash.yaml，也允许后续 backup/import/revalidate 继续处理环境资产。
func validateProxyMaterialsForAdmission(envPath string, profile model.ProfileFile) error {
	if strings.TrimSpace(profile.Proxy.ConfigPath) != profile.Paths.ProxyConfig {
		return invalidError("profile.proxy.configPath 与 profile.paths.proxyConfig 不一致")
	}
	if !profile.Proxy.Enabled {
		return nil
	}
	if err := requirePackageFile(envPath, profile.Paths.ProxyConfig); err != nil {
		return invalidError(err.Error())
	}
	if _, err := readProxyConfigForAdmission(envPath, profile.Paths.ProxyConfig); err != nil {
		return err
	}
	return nil
}

func validatePackageJSONFile(envPath string, relativePath string, target any) error {
	if err := readPackageJSON(envPath, relativePath, target); err != nil {
		return invalidError(fmt.Sprintf("环境包 JSON 非法 %s: %v", relativePath, err))
	}
	return nil
}

func readProxyConfigForAdmission(envPath string, relativePath string) (string, error) {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return "", invalidError(err.Error())
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", invalidError(fmt.Sprintf("读取代理配置失败: %v", err))
	}
	config := string(bytes)
	if strings.TrimSpace(config) == "" {
		return "", invalidError("proxy/clash.yaml 不能为空")
	}
	if _, err = detectClashTunEnabled(config); err != nil {
		return "", err
	}
	return config, nil
}

// ensureNoDockerConflictForAdmission 在导入、恢复或重建索引前检查 Docker 身份冲突。
//
// 这些动作不启动容器，但如果本机已经存在同 envId label 或同 containerName 的容器，
// 后续 run 会复用或覆盖错误对象，所以必须在准入阶段阻断并交给管理员排查。
func ensureNoDockerConflictForAdmission(envID string, containerName string) error {
	containers, err := edgeService.NewEdgeService().GetDockerContainers()
	if err != nil {
		return remoteError("Docker API 不可达，不能证明环境包容器身份无冲突: " + err.Error())
	}
	envID = strings.TrimSpace(envID)
	containerName = strings.TrimSpace(containerName)
	for _, container := range containers {
		if strings.TrimSpace(container.EnvID) == envID ||
			(container.Labels != nil && strings.TrimSpace(container.Labels["bv.envId"]) == envID) {
			return conflictError("Docker 已存在相同 envId 的容器，不能导入/恢复/重建索引，请管理员排查")
		}
		for _, name := range container.Names {
			if strings.TrimPrefix(strings.TrimSpace(name), "/") == containerName && containerName != "" {
				return conflictError("Docker 已存在相同 containerName 的容器，不能导入/恢复/重建索引，请管理员排查")
			}
		}
	}
	return nil
}
