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

	"gopkg.in/yaml.v3"
)

const containerCDPPort = 9222
const containerVNCPort = 5900
const timezoneRecreateLimit = 3
const hostTunDevicePath = "/dev/net/tun"

var runEnvMu = sync.Mutex{}

type runPackage struct {
	Index            *model.BrowserEnvIndex
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
// - 负责读取 profile/binding/container/proxy/fingerprint-runtime；
// - 负责生成受控 Docker create 参数、启动容器、回写 container.json / profile.lastRuntime / browser_envs；
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

	return runBrowserEnvLocked(envID, param)
}

// runBrowserEnvLocked 执行已经持有生命周期锁的启动流程。
//
// 设计来源：
// - 普通 run 接口和配置修改后的无感重建都需要同一套 Docker create/start 逻辑；
// - 配置修改接口已经持有 runEnvMu，如果再调用 RunBrowserEnv 会死锁；
// - 因此把真正的启动流程拆成 locked helper，外层负责加锁，内部只做环境包到容器的编排。
//
// 维护约束：
// - 这个函数调用前必须已经持有 runEnvMu；
// - 不要从 HTTP 层直接调用它；
// - 它仍然保持 run 的边界：不拉镜像、不选择架构、不删除 browser-data/profile。
func runBrowserEnvLocked(envID string, param *model.RunBrowserEnvRequest) (*model.RunBrowserEnvResponse, error) {
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
	if index.Status == model.BrowserEnvStatusBackedUp || index.Status == model.BrowserEnvStatusArchived {
		return nil, conflictError("环境包当前只有备份包，请先 restore 后再启动")
	}
	if index.Status == model.BrowserEnvStatusError {
		return nil, conflictError("环境包处于 error，普通 run 不能隐式恢复异常环境；请管理员排查文件、Docker 和端口后调用 revalidate")
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
			return finalizeRunPackage(edge, pkg, deviceInfo, existing.ID, true, timezoneRecreateLimit, true)
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
		if _, ok := IsBusinessError(err); ok {
			return nil, err
		}
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
	return finalizeRunPackage(edge, pkg, deviceInfo, created.ID, false, timezoneRecreateLimit, true)
}

// loadRunPackage 读取 run 所需的环境包文件。
//
// 这里从数据库索引拿 envPath，再读取环境包内标准文件，避免绕过 SQLite 直接猜目录。
// 它只做读取和基础一致性校验，不调用 Docker，也不修改文件。
func loadRunPackage(index *model.BrowserEnvIndex) (*runPackage, error) {
	absoluteEnvPath, profile, err := loadPackageProfileFromIndex(index)
	if err != nil {
		return nil, err
	}

	pkg := &runPackage{
		Index:           index,
		Profile:         profile,
		AbsoluteEnvPath: absoluteEnvPath,
	}
	if err := readPackageJSON(absoluteEnvPath, pkg.Profile.Paths.Binding, &pkg.Binding); err != nil {
		return nil, err
	}
	if err := readPackageJSON(absoluteEnvPath, pkg.Profile.Paths.Container, &pkg.Container); err != nil {
		return nil, err
	}

	browserDataPath := filepath.Join(absoluteEnvPath, filepath.FromSlash(pkg.Profile.Paths.BrowserData))
	if stat, err := os.Stat(browserDataPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("browser-data/profile 不存在，环境包核心登录态资产缺失，不能自动创建")
		}
		return nil, fmt.Errorf("读取 browser-data/profile 失败: %w", err)
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("browser-data/profile 不是目录")
	}
	if strings.TrimSpace(pkg.Binding.Storage.HostUserDataDir) != pkg.Profile.Paths.BrowserData {
		return nil, fmt.Errorf("binding.storage.hostUserDataDir 与 profile.paths.browserData 不一致")
	}
	if pkg.Profile.Proxy.Enabled {
		proxyPath, err := safePackagePath(absoluteEnvPath, pkg.Profile.Proxy.ConfigPath)
		if err != nil {
			return nil, err
		}
		bytes, err := os.ReadFile(proxyPath)
		if err != nil {
			return nil, fmt.Errorf("读取代理配置失败: %w", err)
		}
		pkg.ProxyConfig = string(bytes)
	}
	runtimeConfigPath, err := safePackagePath(absoluteEnvPath, pkg.Binding.Fingerprint.RuntimeConfigPath)
	if err != nil {
		return nil, err
	}
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
	if pkg.Binding.Identity.EnvID != pkg.Profile.EnvID ||
		pkg.Binding.Identity.UserID != pkg.Profile.UserID ||
		pkg.Binding.Identity.RPAType != pkg.Profile.RPAType {
		return fmt.Errorf("binding.identity 与 profile 不一致")
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
	identity := buildBindingIdentityFromFacts(pkg.Profile.EnvID, pkg.Profile.UserID, pkg.Profile.RPAType)
	identityHash, err := buildJSONHash(identity)
	if err != nil {
		return fmt.Errorf("重新计算 identityHash 失败: %w", err)
	}
	if identityHash != pkg.Binding.IdentityHash {
		return fmt.Errorf("identityHash 与 binding 不一致")
	}
	if strings.TrimSpace(pkg.Profile.IdentityHash) != "" && identityHash != pkg.Profile.IdentityHash {
		return fmt.Errorf("identityHash 与 profile 不一致")
	}
	return nil
}

// buildBindingIdentityFromFacts 使用环境包稳定事实重建 identityHash 来源结构。
//
// 当前 identityHash 只做 envId/userId/rpaType 一致性摘要。
// timezone、language、screen、proxy、browserDataPath、端口、设备和容器都不参与身份计算。
func buildBindingIdentityFromFacts(envID string, userID string, rpaType string) model.BindingIdentity {
	return model.BindingIdentity{
		EnvID:   envID,
		UserID:  userID,
		RPAType: rpaType,
	}
}

// buildDockerCreateConfig 把环境包转换成 Docker Engine create 请求。
//
// 这里是 run 的核心转换层，但仍然保持受控：
// - 镜像来自 profile.runtime.image，由中心服务端提前按节点架构决定；
// - 端口、挂载、代理、指纹都来自环境包文件；
// - 不接受前端传入任意 HostConfig，避免 run 退化为 Docker 参数透传。
func buildDockerCreateConfig(pkg *runPackage) (*edgeModel.DockerContainerCreateConfig, error) {
	shmSize, err := parseSizeBytes(pkg.Profile.Runtime.ShmSize)
	if err != nil {
		return nil, err
	}
	browserDataPath := filepath.Join(pkg.AbsoluteEnvPath, filepath.FromSlash(pkg.Binding.Storage.HostUserDataDir))
	cdpKey := dockerPortKey(containerCDPPort)
	vncKey := dockerPortKey(containerVNCPort)
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
		// Chromium 容器需要沿用旧 compose 的 seccomp:unconfined。
		//
		// 背景：用户在测试 1.1 镜像时发现容器反复 restarting；日志显示 Chromium
		// `No usable sandbox`。旧 Private_Browser_Control 容器通过 seccomp:unconfined
		// 让 Chromium sandbox 能正常工作，因此 Go 版 run 不能遗漏这个 HostConfig。
		SecurityOpt: []string{"seccomp:unconfined"},
	}
	runtimeProxyConfig := pkg.ProxyConfig
	tunEnabled, err := detectClashTunEnabled(pkg.ProxyConfig)
	if err != nil {
		return nil, err
	}
	if tunEnabled {
		if hostTunDeviceAvailable() {
			hostConfig.CapAdd = []string{"NET_ADMIN"}
			hostConfig.Devices = []edgeModel.DockerContainerDeviceMapping{
				{
					PathOnHost:        "/dev/net/tun",
					PathInContainer:   "/dev/net/tun",
					CgroupPermissions: "rwm",
				},
			}
		} else {
			return nil, fmt.Errorf("proxy/clash.yaml 启用了 tun.enable=true，但宿主机缺少 %s；请为 Client/浏览器容器提供 NET_ADMIN 和 /dev/net/tun，或在配置中显式关闭 TUN 后重新提交代理配置，不能静默降级为非 TUN 运行", hostTunDevicePath)
		}
	}
	envValues, err := buildContainerEnv(pkg, runtimeProxyConfig)
	if err != nil {
		return nil, err
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
// 这些变量对应 Private_Browser_Edge_AMD64/ARM 镜像 entrypoint 里已经验证过的启动参数；
// 第一版只从环境包转换，不允许调用方在 run 请求里覆盖，避免前后端状态不一致。
//
// runtimeProxyConfig 是本次容器实际注入的代理配置。它通常等于 pkg.ProxyConfig；
// 但当模板启用了 tun 而宿主机没有 /dev/net/tun 时，run 会临时把注入配置里的 tun.enable 改成 false，
// 避免镜像入口脚本直接退出，同时不改写环境包磁盘上的 proxy/clash.yaml。
//
// 端口边界也在这里固定：
// - profile.ports.vnc 是宿主机发布端口，按 9100 + envSequence 分配；
// - 容器内 x11vnc 固定监听 5900，不随宿主端口变化；
// - Docker PortBindings 负责 910x:5900，不能把 VNC_PORT 注入成宿主端口。
func buildContainerEnv(pkg *runPackage, runtimeProxyConfig string) ([]string, error) {
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
		"VNC_PORT=" + strconv.Itoa(containerVNCPort),
	}
	if pkg.Profile.Proxy.Enabled {
		envValues = append(envValues,
			"ENABLE_PROXY=true",
			"MIHOMO_CONFIG_BASE64="+base64.StdEncoding.EncodeToString([]byte(runtimeProxyConfig)),
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
func finalizeRunPackage(edge *edgeService.Service, pkg *runPackage, deviceInfo *edgeModel.DeviceInfo, containerID string, reused bool, remainingTimezoneRecreates int, shouldProbeTimezone bool) (*model.RunBrowserEnvResponse, error) {
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

	pkg.Profile.LastRuntime = model.PackageLastRuntime{
		DeviceArch:    &deviceArch,
		DockerAPIURL:  &dockerAPIURL,
		ContainerID:   &containerID,
		ContainerName: &pkg.Container.ContainerName,
		LastStartedAt: &now,
	}
	pkg.Profile.Metadata.UpdatedAt = now

	if err := writeJSONFile(filepath.Join(pkg.AbsoluteEnvPath, filepath.FromSlash(pkg.Profile.Paths.Container)), pkg.Container); err != nil {
		return nil, internalError(err.Error())
	}
	if err := writePackageJSON(pkg.AbsoluteEnvPath, pkg.Profile.Paths.Profile, pkg.Profile); err != nil {
		return nil, internalError(err.Error())
	}
	timezoneStatus := ""
	timezoneError := ""
	if !shouldProbeTimezone {
		timezoneStatus = "verified"
	}
	if shouldProbeTimezone {
		timezoneStatus = "verified"
		probeResult, err := applyContainerTimezoneProbe(pkg, containerID)
		if err != nil {
			return nil, remoteError(err.Error())
		}
		if probeResult != nil && probeResult.ProbeFailed {
			timezoneStatus = "failed"
			timezoneError = probeResult.Error
			message := "timezone/代理出口探测失败，容器可能仍在 running，但环境因网络指纹未确认不可用: " + timezoneError
			_ = updateRunErrorWithRuntime(pkg, message, containerID)
			return nil, remoteError(message)
		}
		if probeResult != nil && probeResult.NeedsContainerRecreate {
			if remainingTimezoneRecreates <= 0 {
				message := "timezone 探测结果在多次重建后仍变化，已停止继续自动重建"
				_ = updateRunErrorWithRuntime(pkg, message, containerID)
				return nil, remoteError(message)
			}
			return recreateContainerAfterTimezoneProbe(edge, pkg, deviceInfo, containerID, remainingTimezoneRecreates-1)
		}
	}

	containerName := pkg.Container.ContainerName
	if err := browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(context.Background(), &model.BrowserEnvRuntimeUpdate{
		EnvID:           pkg.Profile.EnvID,
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
		EnvID:          pkg.Profile.EnvID,
		ContainerID:    containerID,
		ContainerName:  pkg.Container.ContainerName,
		Image:          pkg.Profile.Runtime.Image,
		Status:         model.BrowserEnvStatusRunning,
		Ports:          pkg.Profile.Ports,
		CDPURL:         "http://" + publishedPortAddressForService(pkg.Profile.Ports.CDP),
		VNCURL:         "vnc://" + publishedPortAddressForService(pkg.Profile.Ports.VNC),
		DockerAPIURL:   dockerAPIURL,
		DeviceArch:     deviceArch,
		TimezoneStatus: timezoneStatus,
		TimezoneError:  timezoneError,
		StartedAt:      now,
		Reused:         reused,
	}, nil
}

// recreateContainerAfterTimezoneProbe 在 timezone 探测回写后重建容器。
//
// 设计来源：
// - TZ 是 Docker 容器启动环境变量，浏览器进程启动后再改 profile.json 不会自动生效；
// - 用户实测发现必须等 Clash/TUN 接管后才能得到真实出口 timezone；
// - 因此容器会先用于初始化代理链路和探测真实 timezone，若探测值变化，必须用新 TZ 重建容器。
//
// 维护约束：
// - 只允许有限次数重建，避免 provider 抖动导致无限循环；
// - 不删除 browser-data/profile，只删除本次运行容器；
// - 重建后的容器直接使用前一次探测回写的 TZ，避免 provider/CDP 二次等待拖慢 HTTP 响应。
func recreateContainerAfterTimezoneProbe(edge *edgeService.Service, pkg *runPackage, deviceInfo *edgeModel.DeviceInfo, oldContainerID string, remainingTimezoneRecreates int) (*model.RunBrowserEnvResponse, error) {
	if edge == nil {
		edge = edgeService.NewEdgeService()
	}
	if err := edge.RemoveDockerContainer(oldContainerID, true); err != nil {
		_ = updateRunErrorWithRuntime(pkg, err.Error(), oldContainerID)
		return nil, remoteError(err.Error())
	}
	reloaded, err := loadRunPackage(pkg.Index)
	if err != nil {
		_ = updateRunError(pkg.Profile.EnvID, err.Error())
		return nil, internalError(err.Error())
	}
	if err = ensureRunPortsAvailable(reloaded.Profile.Ports); err != nil {
		_ = updateRunError(pkg.Profile.EnvID, err.Error())
		return nil, conflictError(err.Error())
	}
	createConfig, err := buildDockerCreateConfig(reloaded)
	if err != nil {
		_ = updateRunError(pkg.Profile.EnvID, err.Error())
		if _, ok := IsBusinessError(err); ok {
			return nil, err
		}
		return nil, internalError(err.Error())
	}
	created, err := edge.CreateDockerContainer(reloaded.Container.ContainerName, createConfig)
	if err != nil {
		_ = updateRunError(pkg.Profile.EnvID, err.Error())
		return nil, remoteError(err.Error())
	}
	if _, err = edge.StartDockerContainer(created.ID); err != nil {
		_ = updateRunError(pkg.Profile.EnvID, err.Error())
		return nil, remoteError(err.Error())
	}
	return finalizeRunPackage(edge, reloaded, deviceInfo, created.ID, false, remainingTimezoneRecreates, false)
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

// detectClashTunEnabled 判断代理配置是否启用了 tun。
//
// 设计来源：
// - 用户指出 clash/mihomo 配置里 `tun.enable=true` 时，容器必须具备 NET_ADMIN 和 /dev/net/tun；
// - 早期正则判断容易被注释、缩进或其它 enable 字段误伤，因此这里改用 YAML 结构化解析；
// - 只有明确开启 TUN 时才提升容器权限，避免普通 mixed-port 代理配置拿到不必要的宿主机能力。
//
// 职责边界：
// - 只判断配置是否需要 TUN，不修改 YAML、不校验完整 clash 语义；
// - YAML 无法解析时返回参数错误，让调用方知道当前配置连 TUN 能力判断都无法安全完成。
func detectClashTunEnabled(configText string) (bool, error) {
	if strings.TrimSpace(configText) == "" {
		return false, nil
	}
	var payload struct {
		Tun struct {
			Enable bool `yaml:"enable"`
		} `yaml:"tun"`
	}
	if err := yaml.Unmarshal([]byte(configText), &payload); err != nil {
		return false, invalidError("代理配置 YAML 无法解析，不能判断 tun.enable: " + err.Error())
	}
	return payload.Tun.Enable, nil
}

// disableClashTunForRuntime 只在本次 Docker 环境变量里关闭 tun.enable。
//
// 设计来源：
//   - 当前代理模板普遍带 `tun.enable=true`，但 Mac / Docker Desktop 常常没有可挂载的 /dev/net/tun；
//   - 浏览器镜像入口脚本会主动拒绝 “tun.enable=true 但无 TUN 设备” 的组合；
//   - 因此边缘服务在无 TUN 节点上做运行时降级：注入容器的 MIHOMO_CONFIG_BASE64 使用 tun.enable=false，
//     让 mixed-port + 浏览器代理链路先跑通；磁盘上的 proxy/clash.yaml 保持原样，便于迁移到 Linux 节点后自动恢复 TUN。
//
// 职责边界：
// - 只修改 YAML 顶层 tun.enable，不改变代理节点、规则、DNS、mode 等其它字段；
// - 只用于容器启动环境变量，不写回 profile/proxy 文件，不改变 identityHash；
// - 如果 YAML 本身无法解析，返回参数错误，避免把坏配置继续注入镜像。
func disableClashTunForRuntime(configText string) (string, error) {
	if strings.TrimSpace(configText) == "" {
		return configText, nil
	}
	var payload map[string]any
	if err := yaml.Unmarshal([]byte(configText), &payload); err != nil {
		return "", invalidError("代理配置 YAML 无法解析，不能运行时关闭 tun.enable: " + err.Error())
	}
	rawTun, ok := payload["tun"]
	if !ok {
		return configText, nil
	}
	tun, ok := rawTun.(map[string]any)
	if !ok {
		return "", invalidError("代理配置 tun 字段不是 YAML object，不能运行时关闭 tun.enable")
	}
	tun["enable"] = false
	payload["tun"] = tun
	bytes, err := yaml.Marshal(payload)
	if err != nil {
		return "", internalError("生成运行时代理配置失败: " + err.Error())
	}
	return string(bytes), nil
}

// hostTunDeviceAvailable 检查当前宿主机是否具备 TUN 设备。
//
// 设计来源：
// - 代理配置模板通常都会带 `tun.enable=true`，所以不能把缺少 /dev/net/tun 当成硬阻断；
// - Linux 商用节点具备 /dev/net/tun 时，自动追加 NET_ADMIN 和设备挂载，获得完整 TUN/DNS 能力；
// - Mac / Docker Desktop 或未加载 tun 的开发节点则降级继续启动，容器仍可依赖 mixed-port + 浏览器代理链路跑通。
//
// 职责边界：
// - 这里只决定是否追加 Docker HostConfig 能力，不改写用户传入的 clash.yaml；
// - 不返回业务错误，避免通用代理模板在无 TUN 节点上无法启动；
// - 后续如果中心端要强制商用节点必须具备 TUN，应新增节点能力校验策略，而不是在本机 run 链路硬失败。
func hostTunDeviceAvailable() bool {
	stat, err := os.Stat(hostTunDevicePath)
	if err != nil {
		return false
	}
	if stat.IsDir() || stat.Mode()&os.ModeDevice == 0 {
		return false
	}
	return true
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
