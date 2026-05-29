package BrowserEnv

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

const containerCDPPort = 9222

var runEnvMu = sync.Mutex{}

type runPackage struct {
	Index            *model.BrowserEnvIndex
	Manifest         model.ManifestFile
	Profile          model.ProfileFile
	Binding          model.BindingFile
	Container        model.ContainerFile
	ProxyConfig      string
	RuntimeConfigRaw []byte
	AbsoluteEnvPath  string
}

// RunBrowserEnv 把本地环境包恢复成运行中的 Docker 浏览器容器。
//
// 设计来源：
// - 当前项目已经先完成“服务端下发 profile -> 边缘服务保存环境包 -> SQLite 索引”的链路；
// - 用户进一步确认镜像由中心服务端根据节点 CPU 决策，边缘服务只执行 profile.runtime.image；
// - 因此 run 不做镜像架构决策，也不接受前端透传 Docker 参数，只围绕 envId 读取环境包并创建/启动容器。
//
// 职责边界：
// - 负责读取 manifest/profile/binding/container/proxy/fingerprint-runtime；
// - 负责生成受控 Docker create 参数、启动容器、回写 container.json / manifest.lastRuntime / browser_envs；
// - 不负责拉取镜像、不选择 arm/x86 镜像、不删除 browser-data/profile 登录态目录。
func (s *Service) RunBrowserEnv(envID string, param *model.RunBrowserEnvRequest) (*model.RunBrowserEnvResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}
	if param == nil {
		param = &model.RunBrowserEnvRequest{}
	}

	runEnvMu.Lock()
	defer runEnvMu.Unlock()

	handler := browserEnvDao.NewRuntimeModelHandler()
	index, err := handler.GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}
	if index.Status == model.BrowserEnvStatusDeleted {
		return nil, conflictError("环境包已删除，不能启动")
	}

	pkg, err := loadRunPackage(index)
	if err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, internalError(err.Error())
	}

	edge := edgeService.NewEdgeService()
	deviceInfo, err := edge.GetDeviceInfo()
	if err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, remoteError(err.Error())
	}

	existing, err := findContainerByName(edge, pkg.Container.ContainerName)
	if err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, remoteError(err.Error())
	}
	if existing != nil {
		if param.ForceRecreate {
			if err = edge.RemoveDockerContainer(existing.ID, true); err != nil {
				_ = updateRunError(envID, err.Error())
				return nil, remoteError(err.Error())
			}
		} else {
			if _, err = edge.StartDockerContainer(existing.ID); err != nil {
				_ = updateRunError(envID, err.Error())
				return nil, remoteError(err.Error())
			}
			return finalizeRunPackage(pkg, deviceInfo, existing.ID, true)
		}
	}

	if err = ensureImageExists(edge, pkg.Profile.Runtime.Image); err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, conflictError(err.Error())
	}
	if err = ensureRunPortsAvailable(pkg.Profile.Ports); err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, conflictError(err.Error())
	}

	createConfig, err := buildDockerCreateConfig(pkg)
	if err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, internalError(err.Error())
	}
	created, err := edge.CreateDockerContainer(pkg.Container.ContainerName, createConfig)
	if err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, remoteError(err.Error())
	}
	if _, err = edge.StartDockerContainer(created.ID); err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, remoteError(err.Error())
	}
	return finalizeRunPackage(pkg, deviceInfo, created.ID, false)
}

// loadRunPackage 读取 run 所需的环境包文件。
//
// 这里从数据库索引拿 envPath，再读取环境包内标准文件，避免绕过 SQLite 直接猜目录。
// 它只做读取和基础一致性校验，不调用 Docker，也不修改文件。
func loadRunPackage(index *model.BrowserEnvIndex) (*runPackage, error) {
	if index == nil {
		return nil, fmt.Errorf("环境包索引不能为空")
	}
	if Settings.Conf.ProjectRoot == "" {
		return nil, fmt.Errorf("project root 不能为空")
	}
	absoluteEnvPath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(index.EnvPath))
	if stat, err := os.Stat(absoluteEnvPath); err != nil {
		return nil, fmt.Errorf("环境包目录不存在: %w", err)
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("环境包路径不是目录")
	}

	pkg := &runPackage{
		Index:           index,
		AbsoluteEnvPath: absoluteEnvPath,
	}
	if err := readJSONFile(filepath.Join(absoluteEnvPath, "manifest.json"), &pkg.Manifest); err != nil {
		return nil, err
	}
	if pkg.Manifest.EnvID != index.EnvID {
		return nil, fmt.Errorf("manifest.envId 与数据库索引不一致")
	}
	if err := readJSONFile(filepath.Join(absoluteEnvPath, filepath.FromSlash(pkg.Manifest.Paths.Profile)), &pkg.Profile); err != nil {
		return nil, err
	}
	if err := readJSONFile(filepath.Join(absoluteEnvPath, filepath.FromSlash(pkg.Manifest.Paths.Binding)), &pkg.Binding); err != nil {
		return nil, err
	}
	if err := readJSONFile(filepath.Join(absoluteEnvPath, filepath.FromSlash(pkg.Manifest.Paths.Container)), &pkg.Container); err != nil {
		return nil, err
	}

	browserDataPath := filepath.Join(absoluteEnvPath, filepath.FromSlash(pkg.Binding.Storage.HostUserDataDir))
	if err := os.MkdirAll(browserDataPath, 0755); err != nil {
		return nil, fmt.Errorf("创建 browser-data/profile 失败: %w", err)
	}
	if pkg.Profile.Proxy.Enabled {
		proxyPath := filepath.Join(absoluteEnvPath, filepath.FromSlash(pkg.Profile.Proxy.ConfigPath))
		bytes, err := os.ReadFile(proxyPath)
		if err != nil {
			return nil, fmt.Errorf("读取代理配置失败: %w", err)
		}
		pkg.ProxyConfig = string(bytes)
	}
	runtimeConfigPath := filepath.Join(absoluteEnvPath, filepath.FromSlash(pkg.Binding.Fingerprint.RuntimeConfigPath))
	if bytes, err := os.ReadFile(runtimeConfigPath); err == nil {
		pkg.RuntimeConfigRaw = bytes
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取指纹 runtime-config 失败: %w", err)
	}
	if err := validateRunPackage(pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

// validateRunPackage 校验环境包核心文件的一致性。
//
// run 不能把一个损坏或身份不一致的环境包启动起来；
// 这里重点检查 envId、镜像、端口、binding 存储路径和 identityHash，避免后续误用登录态目录。
func validateRunPackage(pkg *runPackage) error {
	if pkg.Profile.EnvID != pkg.Manifest.EnvID || pkg.Binding.Identity.UserID != pkg.Manifest.UserID {
		return fmt.Errorf("profile/binding 与 manifest 不一致")
	}
	if strings.TrimSpace(pkg.Profile.Runtime.Image) == "" {
		return fmt.Errorf("profile.runtime.image 不能为空")
	}
	if pkg.Profile.Ports.CDP <= 0 || pkg.Profile.Ports.VNC <= 0 {
		return fmt.Errorf("profile.ports.cdp/vnc 必须为正整数")
	}
	if strings.TrimSpace(pkg.Binding.Storage.HostUserDataDir) == "" ||
		strings.TrimSpace(pkg.Binding.Storage.ContainerUserDataDir) == "" {
		return fmt.Errorf("binding.storage 不能为空")
	}
	proxyHash := buildTextHash(pkg.ProxyConfig)
	identity := buildBindingIdentityFromProfile(pkg.Manifest.UserID, pkg.Profile, pkg.Manifest.Paths, proxyHash)
	identityHash, err := buildJSONHash(identity)
	if err != nil {
		return fmt.Errorf("重新计算 identityHash 失败: %w", err)
	}
	if identityHash != pkg.Binding.IdentityHash {
		return fmt.Errorf("identityHash 与 binding 不一致")
	}
	return nil
}

// buildBindingIdentityFromProfile 使用 profile 重建 identityHash 来源结构。
//
// 创建环境包和 run 前校验必须使用同一套稳定字段：
// userId/rpaType/timezone/language/screen/proxyConfigHash/browserDataPath。
// 端口、容器 ID、Docker API、设备架构不允许进入 identityHash。
func buildBindingIdentityFromProfile(userID string, profile model.ProfileFile, paths model.ManifestPaths, proxyConfigHash string) model.BindingIdentity {
	return model.BindingIdentity{
		UserID:   userID,
		RPAType:  profile.RPAType,
		Timezone: profile.Environment.Timezone,
		Language: profile.Environment.Language,
		Screen: model.BindingIdentityScreen{
			Width:  profile.Environment.Screen.Width,
			Height: profile.Environment.Screen.Height,
		},
		Proxy: model.BindingIdentityProxy{
			Type:       profile.Proxy.Type,
			ConfigHash: proxyConfigHash,
		},
		BrowserDataPath: paths.BrowserData,
	}
}

// buildDockerCreateConfig 把环境包转换成 Docker Engine create 请求。
//
// 这里是 run 的核心转换层，但仍然保持受控：
// - 镜像来自 profile.runtime.image，由中心服务端提前按节点架构决定；
// - 端口、挂载、代理、指纹都来自环境包文件；
// - 不接受前端传入任意 HostConfig，避免 run 退化为 Docker 参数透传。
func buildDockerCreateConfig(pkg *runPackage) (*edgeModel.DockerContainerCreateConfig, error) {
	envValues, err := buildContainerEnv(pkg)
	if err != nil {
		return nil, err
	}
	shmSize, err := parseSizeBytes(pkg.Profile.Runtime.ShmSize)
	if err != nil {
		return nil, err
	}
	browserDataPath := filepath.Join(pkg.AbsoluteEnvPath, filepath.FromSlash(pkg.Binding.Storage.HostUserDataDir))
	cdpKey := dockerPortKey(containerCDPPort)
	vncKey := dockerPortKey(pkg.Profile.Ports.VNC)
	exposedPorts := map[string]struct{}{
		cdpKey: {},
		vncKey: {},
	}
	portBindings := map[string][]edgeModel.DockerPortBinding{
		cdpKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(pkg.Profile.Ports.CDP)}},
		vncKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(pkg.Profile.Ports.VNC)}},
	}

	hostConfig := edgeModel.DockerContainerHostConfig{
		Binds: []string{
			browserDataPath + ":" + pkg.Binding.Storage.ContainerUserDataDir,
		},
		PortBindings: portBindings,
		RestartPolicy: edgeModel.DockerContainerRestartPolicy{
			Name: "unless-stopped",
		},
		ShmSize: shmSize,
	}
	if isTunEnabled(pkg.ProxyConfig) {
		hostConfig.CapAdd = []string{"NET_ADMIN"}
		hostConfig.Devices = []edgeModel.DockerContainerDeviceMapping{
			{
				PathOnHost:        "/dev/net/tun",
				PathInContainer:   "/dev/net/tun",
				CgroupPermissions: "rwm",
			},
		}
	}

	return &edgeModel.DockerContainerCreateConfig{
		Image:        pkg.Profile.Runtime.Image,
		Env:          envValues,
		Labels:       pkg.Container.Labels,
		ExposedPorts: exposedPorts,
		HostConfig:   hostConfig,
	}, nil
}

// buildContainerEnv 生成容器启动环境变量。
//
// 这些变量对应 Private_Browser_Control 镜像 entrypoint 里已经验证过的启动参数；
// 第一版只从环境包转换，不允许调用方在 run 请求里覆盖，避免前后端状态不一致。
func buildContainerEnv(pkg *runPackage) ([]string, error) {
	envValues := []string{
		"TZ=" + pkg.Profile.Environment.Timezone,
		"BROWSER_LANG=" + pkg.Profile.Environment.Language,
		"SCREEN_WIDTH=" + strconv.Itoa(pkg.Profile.Environment.Screen.Width),
		"SCREEN_HEIGHT=" + strconv.Itoa(pkg.Profile.Environment.Screen.Height),
		"SCREEN_DEPTH=" + strconv.Itoa(pkg.Profile.Environment.Screen.Depth),
		"DEBUG_PORT=" + strconv.Itoa(containerCDPPort),
		"USER_DATA_DIR=" + pkg.Binding.Storage.ContainerUserDataDir,
		"START_URL=" + pkg.Profile.Runtime.StartupURL,
		"ENABLE_VNC=" + strconv.FormatBool(pkg.Profile.Runtime.EnableVNC),
		"VNC_PORT=" + strconv.Itoa(pkg.Profile.Ports.VNC),
	}
	if pkg.Profile.Proxy.Enabled {
		envValues = append(envValues,
			"ENABLE_CLASH_VERGE=true",
			"CLASH_VERGE_CONFIG_BASE64="+base64.StdEncoding.EncodeToString([]byte(pkg.ProxyConfig)),
		)
	}
	runtimeConfig := strings.TrimSpace(string(pkg.RuntimeConfigRaw))
	if runtimeConfig != "" && runtimeConfig != "{}" {
		envValues = append(envValues, "FINGERPRINT_RUNTIME_CONFIG_BASE64="+base64.StdEncoding.EncodeToString(pkg.RuntimeConfigRaw))
		if userAgent := extractRuntimeUserAgent(pkg.RuntimeConfigRaw); userAgent != "" {
			envValues = append(envValues, "BROWSER_USER_AGENT="+userAgent)
		}
	}
	return envValues, nil
}

// findContainerByName 在本机 Docker 容器列表里查找同名容器。
//
// Docker create 如果同名容器已存在会冲突；run 的规则是默认复用并 start，
// 只有 forceRecreate=true 才删除重建，因此需要先按 container.json.containerName 查一次。
func findContainerByName(edge *edgeService.Service, containerName string) (*edgeModel.DockerContainer, error) {
	containers, err := edge.GetDockerContainers()
	if err != nil {
		return nil, err
	}
	for _, container := range containers {
		for _, name := range container.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				found := container
				return &found, nil
			}
		}
	}
	return nil, nil
}

// ensureImageExists 检查 profile.runtime.image 是否已经在本机 Docker 中存在。
//
// run 不负责拉镜像；如果镜像不存在，返回明确冲突，让调用方先执行 pull-image。
func ensureImageExists(edge *edgeService.Service, image string) error {
	images, err := edge.GetDockerImages()
	if err != nil {
		return err
	}
	for _, item := range images {
		if item.ID == image {
			return nil
		}
		for _, tag := range item.RepoTags {
			if tag == image {
				return nil
			}
		}
		for _, digest := range item.RepoDigests {
			if digest == image {
				return nil
			}
		}
	}
	return fmt.Errorf("镜像未拉取: %s", image)
}

// ensureRunPortsAvailable 检查本机 CDP/VNC 端口是否可绑定。
//
// 端口是 envSequence 推导出来的本机运行资源；run 前必须挡住端口冲突，
// 否则 Docker create 阶段才失败，错误会更难定位。
func ensureRunPortsAvailable(ports model.BrowserEnvPorts) error {
	if err := ensureTCPPortAvailable(ports.CDP); err != nil {
		return fmt.Errorf("CDP 端口不可用 %d: %w", ports.CDP, err)
	}
	if err := ensureTCPPortAvailable(ports.VNC); err != nil {
		return fmt.Errorf("VNC 端口不可用 %d: %w", ports.VNC, err)
	}
	return nil
}

func ensureTCPPortAvailable(port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	return listener.Close()
}

// finalizeRunPackage 在 Docker 启动成功后回写文件和数据库。
//
// 这里写入的都是运行态字段：containerId、Docker API、deviceArch、lastStartedAt。
// 它们不参与 identityHash，因此不会破坏账号环境原子绑定。
func finalizeRunPackage(pkg *runPackage, deviceInfo *edgeModel.DeviceInfo, containerID string, reused bool) (*model.RunBrowserEnvResponse, error) {
	now := time.Now().Unix()
	deviceArch := deviceInfo.DeviceArch
	dockerAPIURL := deviceInfo.DockerAPIURL

	pkg.Container.ContainerID = &containerID
	pkg.Container.Image = pkg.Profile.Runtime.Image
	pkg.Container.Status = model.BrowserEnvStatusRunning
	pkg.Container.Ports = pkg.Profile.Ports
	pkg.Container.Docker.APIURL = dockerAPIURL
	pkg.Container.Docker.DeviceArch = &deviceArch
	pkg.Container.StartedAt = &now
	pkg.Container.StoppedAt = nil
	pkg.Container.UpdatedAt = now

	pkg.Manifest.LastRuntime = model.ManifestLastRuntime{
		DeviceArch:    &deviceArch,
		DockerAPIURL:  &dockerAPIURL,
		ContainerID:   &containerID,
		ContainerName: &pkg.Container.ContainerName,
		LastStartedAt: &now,
	}
	pkg.Manifest.UpdatedAt = now

	if err := writeJSONFile(filepath.Join(pkg.AbsoluteEnvPath, filepath.FromSlash(pkg.Manifest.Paths.Container)), pkg.Container); err != nil {
		return nil, internalError(err.Error())
	}
	if err := writeJSONFile(filepath.Join(pkg.AbsoluteEnvPath, "manifest.json"), pkg.Manifest); err != nil {
		return nil, internalError(err.Error())
	}

	containerName := pkg.Container.ContainerName
	if err := browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(context.Background(), &model.BrowserEnvRuntimeUpdate{
		EnvID:           pkg.Manifest.EnvID,
		Status:          model.BrowserEnvStatusRunning,
		ContainerID:     &containerID,
		ContainerName:   &containerName,
		ContainerStatus: model.BrowserEnvStatusRunning,
		MonitorStatus:   model.BrowserEnvMonitorStatusUnknown,
		UpdatedAt:       now,
		LastStartedAt:   &now,
		LastStoppedAt:   pkg.Index.LastStoppedAt,
		LastCheckedAt:   &now,
	}); err != nil {
		return nil, internalError(err.Error())
	}

	return &model.RunBrowserEnvResponse{
		EnvID:         pkg.Manifest.EnvID,
		ContainerID:   containerID,
		ContainerName: pkg.Container.ContainerName,
		Image:         pkg.Profile.Runtime.Image,
		Status:        model.BrowserEnvStatusRunning,
		Ports:         pkg.Profile.Ports,
		CDPURL:        fmt.Sprintf("http://127.0.0.1:%d", pkg.Profile.Ports.CDP),
		VNCURL:        fmt.Sprintf("vnc://127.0.0.1:%d", pkg.Profile.Ports.VNC),
		DockerAPIURL:  dockerAPIURL,
		DeviceArch:    deviceArch,
		StartedAt:     now,
		Reused:        reused,
	}, nil
}

// updateRunError 把最近一次 run 失败写入 browser_envs。
//
// 这样前端列表能立刻看到 error 和 lastError；它不会删除环境包文件，也不会碰 browser-data 登录态。
func updateRunError(envID string, message string) error {
	now := time.Now().Unix()
	lastError := truncateRunError(message)
	handler := browserEnvDao.NewRuntimeModelHandler()
	index, err := handler.GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		return err
	}
	return handler.UpdateBrowserEnvRuntime(context.Background(), &model.BrowserEnvRuntimeUpdate{
		EnvID:           envID,
		Status:          model.BrowserEnvStatusError,
		ContainerID:     index.ContainerID,
		ContainerName:   index.ContainerName,
		ContainerStatus: index.ContainerStatus,
		MonitorStatus:   index.MonitorStatus,
		LastError:       &lastError,
		UpdatedAt:       now,
		LastStartedAt:   index.LastStartedAt,
		LastStoppedAt:   index.LastStoppedAt,
		LastCheckedAt:   &now,
	})
}

func truncateRunError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 500 {
		return message
	}
	return message[:500]
}

func dockerPortKey(port int) string {
	return strconv.Itoa(port) + "/tcp"
}

// isTunEnabled 判断代理配置是否启用了 tun。
//
// 这沿用 Node 版 compose 生成脚本的判断思路：只有 clash 配置里明确 tun.enable=true，
// 才给容器追加 NET_ADMIN 和 /dev/net/tun，避免普通代理配置拿到不必要的宿主机能力。
func isTunEnabled(configText string) bool {
	pattern := regexp.MustCompile(`(?m)^\s*tun:\s*[\s\S]*?^\s*enable:\s*true\s*$`)
	return pattern.MatchString(configText)
}

// parseSizeBytes 把 profile.runtime.shmSize 转成 Docker API 需要的字节数。
//
// profile 里保留 "1g" 这种人可读写法；Docker Engine API 的 ShmSize 需要 int64 字节，
// 因此转换集中在 run 阶段，不把字节数写回环境包身份字段。
func parseSizeBytes(raw string) (int64, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return 0, nil
	}
	matches := regexp.MustCompile(`^(\d+)([kmgt]?b?|)$`).FindStringSubmatch(value)
	if matches == nil {
		return 0, fmt.Errorf("runtime.shmSize 格式不支持: %s", raw)
	}
	number, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, err
	}
	switch matches[2] {
	case "", "b":
		return number, nil
	case "k", "kb":
		return number * 1024, nil
	case "m", "mb":
		return number * 1024 * 1024, nil
	case "g", "gb":
		return number * 1024 * 1024 * 1024, nil
	case "t", "tb":
		return number * 1024 * 1024 * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("runtime.shmSize 单位不支持: %s", raw)
	}
}

func extractRuntimeUserAgent(raw []byte) string {
	var payload struct {
		UserAgent string `json:"userAgent"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.UserAgent)
}

func readJSONFile(path string, target any) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 JSON 文件失败 %s: %w", path, err)
	}
	if err = json.Unmarshal(bytes, target); err != nil {
		return fmt.Errorf("解析 JSON 文件失败 %s: %w", path, err)
	}
	return nil
}
