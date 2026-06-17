package BrowserEnv

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	model "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	slotModel "private_browser_client/Models/Slot"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"

	"gopkg.in/yaml.v3"
)

const hostTunDevicePath = "/dev/net/tun"

var slotRuntimeRebuilder = rebuildSlotRuntimeForPackage

// rebuildSlotRuntimeForPackage 把 slot 常驻容器切换成“当前环境包的真实运行现场”。
//
// 设计来源：
// - 当前用户已经明确指出：run 成功但账号数据没有恢复，说明原来的 run 只改了状态，没有加载环境包资产；
// - slot 仍然是 WebVNC/CDP 的稳定入口，因此这里不新建一套外部容器名，而是直接重建 slot 当前容器；
// - 这样可以保留 `web-vnc.html?slot=slot001` 这类入口不变，同时让容器内真正加载导入包里的 browser-data/profile。
//
// 职责边界：
// - 负责移除 slot 当前占位容器，并按环境包 profile/binding/proxy 重建运行容器；
// - 负责把 browser-data/profile 绑定到容器内 user-data 目录；
// - 不负责 package/slot 状态机写库，这部分仍由 Package/Runtime/Slot Service 统一收口。
func rebuildSlotRuntimeForPackage(slot *slotModel.Slot, pkg *loadedPackage) (*edgeModel.DockerContainerCreateResult, error) {
	if slot == nil {
		return nil, fmt.Errorf("slot 不能为空")
	}
	if pkg == nil {
		return nil, fmt.Errorf("环境包不能为空")
	}
	if slot.CDPPort == nil || slot.VNCPort == nil {
		return nil, fmt.Errorf("slot 缺少 CDP/VNC 端口，不能加载环境包")
	}
	if strings.TrimSpace(pkg.Binding.Storage.HostUserDataDir) == "" || strings.TrimSpace(pkg.Binding.Storage.ContainerUserDataDir) == "" {
		return nil, fmt.Errorf("binding.storage 不能为空")
	}

	browserDataPath := filepath.Join(pkg.EnvPath, filepath.FromSlash(pkg.Binding.Storage.HostUserDataDir))
	if stat, err := os.Stat(browserDataPath); err != nil {
		return nil, fmt.Errorf("browser-data/profile 缺失: %w", err)
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("browser-data/profile 必须是目录")
	}

	createConfig, err := buildManagedSlotCreateConfig(slot, pkg, browserDataPath)
	if err != nil {
		return nil, err
	}

	if err = maybeRemoveContainer(slot.ContainerID); err != nil {
		return nil, err
	}

	edge := edgeService.NewEdgeService()
	containerName := managedSlotContainerName(slot)
	created, err := edge.CreateDockerContainer(containerName, createConfig)
	if err != nil && shouldPullImageBeforeRetry(err) {
		if _, pullErr := edge.PullDockerImage(pkg.Profile.Runtime.Image); pullErr != nil {
			return nil, fmt.Errorf("slot runtime pull image failed: %w", pullErr)
		}
		created, err = edge.CreateDockerContainer(containerName, createConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("slot runtime create container failed: %w", err)
	}
	if _, err = edge.StartDockerContainer(created.ID); err != nil {
		_ = edge.RemoveDockerContainer(created.ID, true)
		return nil, fmt.Errorf("slot runtime start container failed: %w", err)
	}

	slot.ContainerID = optionalString(created.ID)
	slot.ContainerName = optionalString(containerName)
	slot.RuntimeImage = optionalString(pkg.Profile.Runtime.Image)
	slot.ContainerStatus = optionalString(model.ContainerStatusRunning)
	slot.LastError = nil
	return created, nil
}

// buildManagedSlotCreateConfig 把环境包转换成“slot 入口容器”的 Docker create 请求。
//
// 这里故意保留 slot 自己的宿主机端口和容器名，只替换运行镜像、挂载和环境变量。
// 这样 WebVNC/CDP 的外部入口保持稳定，而容器内部真正消费导入包资产。
func buildManagedSlotCreateConfig(slot *slotModel.Slot, pkg *loadedPackage, browserDataPath string) (*edgeModel.DockerContainerCreateConfig, error) {
	internalCDPPort, internalVNCPort := currentInternalBrowserPorts()
	shmSize, err := parseSizeBytes(pkg.Profile.Runtime.ShmSize)
	if err != nil {
		return nil, err
	}

	cdpKey := dockerPortKey(internalCDPPort)
	vncKey := dockerPortKey(internalVNCPort)
	hostConfig := edgeModel.DockerContainerHostConfig{
		Binds: []string{
			browserDataPath + ":" + pkg.Binding.Storage.ContainerUserDataDir,
		},
		RestartPolicy: edgeModel.DockerContainerRestartPolicy{Name: "unless-stopped"},
		PortBindings: map[string][]edgeModel.DockerPortBinding{
			cdpKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(*slot.CDPPort)}},
			vncKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(*slot.VNCPort)}},
		},
		ShmSize:     shmSize,
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
					PathOnHost:        hostTunDevicePath,
					PathInContainer:   hostTunDevicePath,
					CgroupPermissions: "rwm",
				},
			}
		} else {
			runtimeProxyConfig, err = disableClashTunForRuntime(pkg.ProxyConfig)
			if err != nil {
				return nil, err
			}
		}
	}

	envValues := buildManagedSlotRuntimeEnv(pkg, runtimeProxyConfig, internalCDPPort, internalVNCPort)
	labels := cloneStringMap(pkg.Container.Labels)
	if labels == nil {
		labels = map[string]string{}
	}
	labels["bv.project"] = "private-browser-client"
	labels["bv.role"] = "browser-env"
	labels["bv.slotId"] = slot.SlotID
	labels["bv.envId"] = pkg.Profile.EnvID
	labels["bv.userId"] = pkg.Profile.UserID
	labels["bv.rpaType"] = pkg.Profile.RPAType

	return &edgeModel.DockerContainerCreateConfig{
		Image:  pkg.Profile.Runtime.Image,
		Env:    envValues,
		Labels: labels,
		ExposedPorts: map[string]struct{}{
			cdpKey: {},
			vncKey: {},
		},
		HostConfig: hostConfig,
	}, nil
}

// buildManagedSlotRuntimeEnv 生成当前 slot 容器需要的启动环境变量。
//
// 这里沿用 old 已验证过的镜像入口变量，确保容器真正消费：
// - 时区、语言、屏幕尺寸
// - 用户数据目录
// - 代理 YAML
func buildManagedSlotRuntimeEnv(pkg *loadedPackage, runtimeProxyConfig string, internalCDPPort int, internalVNCPort int) []string {
	envValues := []string{
		"TZ=" + pkg.Profile.Environment.Timezone,
		"BROWSER_LANG=" + pkg.Profile.Environment.Language,
		"SCREEN_WIDTH=" + strconv.Itoa(pkg.Profile.Environment.Screen.Width),
		"SCREEN_HEIGHT=" + strconv.Itoa(pkg.Profile.Environment.Screen.Height),
		"SCREEN_DEPTH=" + strconv.Itoa(pkg.Profile.Environment.Screen.Depth),
		"DEBUG_PORT=" + strconv.Itoa(internalCDPPort),
		"USER_DATA_DIR=" + pkg.Binding.Storage.ContainerUserDataDir,
		"START_URL=" + pkg.Profile.Runtime.StartupURL,
		"ENABLE_VNC=" + strconv.FormatBool(pkg.Profile.Runtime.EnableVNC),
		"VNC_PORT=" + strconv.Itoa(internalVNCPort),
	}
	if pkg.Profile.Proxy.Enabled {
		envValues = append(envValues,
			"ENABLE_PROXY=true",
			"MIHOMO_CONFIG_BASE64="+base64.StdEncoding.EncodeToString([]byte(runtimeProxyConfig)),
		)
	}
	return maybeAppendFingerprintRuntimeEnv(envValues, pkg)
}

// persistRunRuntimeSummary 把本次真实运行现场回写到环境包文件。
//
// 设计来源：
// - profile/container 是环境包本机事实源的一部分；
// - run 成功后如果只改 SQLite，不回写文件，后续 backup/restore/rebuild-index 会丢现场摘要；
// - 因此这里至少把容器 ID、容器名和 lastRuntime 回写到正式文件里。
func persistRunRuntimeSummary(pkg *loadedPackage, slot *slotModel.Slot, containerID string, startedAt int64) error {
	containerName := managedSlotContainerName(slot)
	pkg.Profile.LastRuntime.ContainerID = optionalString(containerID)
	pkg.Profile.LastRuntime.ContainerName = optionalString(containerName)
	pkg.Profile.LastRuntime.LastStartedAt = optionalInt64(startedAt)
	pkg.Profile.LastRuntime.LastStoppedAt = nil
	pkg.Profile.Metadata.UpdatedAt = startedAt

	pkg.Container.ContainerID = optionalString(containerID)
	pkg.Container.ContainerName = containerName
	pkg.Container.Image = pkg.Profile.Runtime.Image
	pkg.Container.Status = model.ContainerStatusRunning
	pkg.Container.StartedAt = optionalInt64(startedAt)
	pkg.Container.StoppedAt = nil
	pkg.Container.UpdatedAt = startedAt

	if err := writePackageJSON(pkg.EnvPath, pkg.Profile.Paths.Profile, pkg.Profile); err != nil {
		return err
	}
	if err := writePackageJSON(pkg.EnvPath, pkg.Profile.Paths.Container, pkg.Container); err != nil {
		return err
	}
	return nil
}

func managedSlotContainerName(slot *slotModel.Slot) string {
	if slot != nil && slot.ContainerName != nil && strings.TrimSpace(*slot.ContainerName) != "" {
		return strings.TrimSpace(*slot.ContainerName)
	}
	prefix := "private-browser-slot"
	if Settings.Conf != nil && Settings.Conf.SlotRuntimeConfig != nil && strings.TrimSpace(Settings.Conf.SlotRuntimeConfig.ContainerNamePrefix) != "" {
		prefix = strings.TrimSpace(Settings.Conf.SlotRuntimeConfig.ContainerNamePrefix)
	}
	return prefix + "-" + strings.TrimSpace(slot.SlotID)
}

func currentInternalBrowserPorts() (int, int) {
	cdpPort := 9222
	vncPort := 5900
	if Settings.Conf != nil && Settings.Conf.SlotRuntimeConfig != nil {
		if Settings.Conf.SlotRuntimeConfig.InternalCDPPort > 0 {
			cdpPort = Settings.Conf.SlotRuntimeConfig.InternalCDPPort
		}
		if Settings.Conf.SlotRuntimeConfig.InternalVNCPort > 0 {
			vncPort = Settings.Conf.SlotRuntimeConfig.InternalVNCPort
		}
	}
	return cdpPort, vncPort
}

func cloneStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func dockerPortKey(port int) string {
	return strconv.Itoa(port) + "/tcp"
}

func shouldPullImageBeforeRetry(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such image") || strings.Contains(message, "pull access denied")
}

// detectClashTunEnabled 只判断代理配置是否显式启用了 tun.enable。
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

// disableClashTunForRuntime 只在本次容器注入配置里关闭 tun.enable。
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

func readRuntimeConfigRaw(pkg *loadedPackage) []byte {
	if pkg == nil {
		return nil
	}
	path, err := safePackagePath(pkg.EnvPath, pkg.Profile.Paths.FingerprintRuntimeConfig)
	if err != nil {
		return nil
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return bytes
}

func maybeAppendFingerprintRuntimeEnv(envValues []string, pkg *loadedPackage) []string {
	raw := strings.TrimSpace(string(readRuntimeConfigRaw(pkg)))
	if raw == "" || raw == "{}" {
		return envValues
	}
	envValues = append(envValues, "FINGERPRINT_RUNTIME_CONFIG_BASE64="+base64.StdEncoding.EncodeToString([]byte(raw)))
	var payload struct {
		UserAgent string `json:"userAgent"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil && strings.TrimSpace(payload.UserAgent) != "" {
		envValues = append(envValues, "BROWSER_USER_AGENT="+strings.TrimSpace(payload.UserAgent))
	}
	return envValues
}
