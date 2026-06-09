package Edge

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	edgeModel "private_browser_client/Models/Edge"
	"private_browser_client/Settings"
)

type Service struct {
	httpClient *http.Client
}

// NewEdgeService 创建边缘服务业务对象。
//
// 当前服务只管理本机 Docker，所以依赖很轻：HTTP client 用来访问 Docker Engine 2375，
// Docker API 地址来自 Settings，而不是从数据库里的节点列表读取。
func NewEdgeService() *Service {
	return &Service{
		httpClient: &http.Client{Timeout: 45 * time.Second},
	}
}

// GetDeviceInfo 获取本机设备能力信息。
//
// 职责边界：
// - 只访问本机 Docker Engine API；
// - 只返回本机设备、Docker 版本、镜像/容器数量等摘要；
// - 不写数据库、不生成设备编号、不处理用户归属。
func (s *Service) GetDeviceInfo() (*edgeModel.DeviceInfo, error) {
	return s.probeDockerEngine()
}

// GetDockerStatus 获取本机 Docker 状态摘要。
//
// 这个接口给前端或未来中心服务端快速判断边缘节点是否具备 Docker 管理能力。
// 如果 Docker 不可达，这里返回错误，由 HTTP 层统一映射成响应。
func (s *Service) GetDockerStatus() (*edgeModel.DockerStatus, error) {
	info, err := s.probeDockerEngine()
	if err != nil {
		return nil, err
	}
	return &edgeModel.DockerStatus{
		DockerAPIURL:    info.DockerAPIURL,
		Status:          info.LastDockerStatus,
		Message:         info.LastDockerMessage,
		ImagesCount:     info.LastImagesCount,
		ContainersCount: info.LastContainersCount,
		CheckedAt:       info.CheckedAt,
	}, nil
}

// GetDockerImages 获取本机 Docker 镜像列表。
//
// 设计来源：
// - 边缘服务后续要负责本机镜像和容器管理；
// - 镜像列表是拉镜像、删镜像、按架构选择浏览器镜像前的只读基础能力；
// - 这里不写数据库，也不把镜像归属到用户或节点，中心服务端如需缓存应自行同步。
func (s *Service) GetDockerImages() ([]edgeModel.DockerImage, error) {
	dockerAPIURL := normalizeDockerAPIURL(Settings.Conf.DockerConfig.APIURL)
	if dockerAPIURL == "" {
		return nil, fmt.Errorf("docker api url 不能为空")
	}

	var rawList []edgeModel.DockerEngineImageResponse
	if err := s.fetchJSON(dockerAPIURL, http.MethodGet, "/images/json", nil, &rawList); err != nil {
		return nil, fmt.Errorf("docker api images failed: %w", err)
	}

	images := make([]edgeModel.DockerImage, 0, len(rawList))
	for _, raw := range rawList {
		images = append(images, edgeModel.DockerImage{
			ID:          raw.ID,
			RepoTags:    normalizeStringSlice(raw.RepoTags),
			RepoDigests: normalizeStringSlice(raw.RepoDigests),
			Created:     raw.Created,
			Size:        raw.Size,
			VirtualSize: raw.VirtualSize,
			SharedSize:  raw.SharedSize,
			Containers:  raw.Containers,
		})
	}
	return images, nil
}

// GetDockerContainers 获取本项目相关的本机 Docker 容器列表。
//
// 设计来源：
// - 用户测试发现直接返回本机 Docker 全量容器会混入其他项目，例如数据库、旧控制台或无关业务服务；
// - 当前边缘服务只应该关心两个类型：Private_Browser_Client 边缘服务自身容器、由本项目创建的浏览器容器；
// - 因此这里仍然请求 `/containers/json?all=true`，但会在 Service 层按 label/name/image 过滤并补 projectRole。
//
// 维护边界：
// - 这个接口不再作为通用 Docker 容器浏览器；
// - 浏览器容器优先通过 `bv.project/bv.role/bv.envId` 识别，兼容早期只有 `bv.envId` 的测试容器；
// - 边缘服务容器优先通过 `private-browser-client` 名称、镜像或 label 识别。
func (s *Service) GetDockerContainers() ([]edgeModel.DockerContainer, error) {
	dockerAPIURL := normalizeDockerAPIURL(Settings.Conf.DockerConfig.APIURL)
	if dockerAPIURL == "" {
		return nil, fmt.Errorf("docker api url 不能为空")
	}

	var rawList []edgeModel.DockerEngineContainerResponse
	if err := s.fetchJSON(dockerAPIURL, http.MethodGet, "/containers/json?all=true", nil, &rawList); err != nil {
		return nil, fmt.Errorf("docker api containers failed: %w", err)
	}

	containers := make([]edgeModel.DockerContainer, 0, len(rawList))
	for _, raw := range rawList {
		container := edgeModel.DockerContainer{
			ID:      raw.ID,
			Names:   normalizeStringSlice(raw.Names),
			Image:   raw.Image,
			ImageID: raw.ImageID,
			Command: raw.Command,
			Created: raw.Created,
			Ports:   buildContainerPorts(raw.Ports),
			Labels:  normalizeStringMap(raw.Labels),
			State:   raw.State,
			Status:  raw.Status,
		}
		if !applyProjectContainerMetadata(&container) {
			continue
		}
		containers = append(containers, container)
	}
	return containers, nil
}

// CreateDockerContainer 在本机 Docker 中创建容器。
//
// 设计来源：
// - BrowserEnv.run 需要把环境包恢复成容器，但不应该直接在 BrowserEnv 里散写 Docker HTTP 细节；
// - Edge Service 继续承担“本机 Docker API 适配层”的职责，BrowserEnv 只负责生成受控 create 配置。
//
// 维护边界：
// - 这里只转发已经构造好的 Docker create 请求，不做环境包读取、不判断账号归属；
// - name 由上层按 envId 生成，不能让前端随意指定；
// - Docker 返回的冲突、镜像不存在等错误原样归一化给上层处理。
func (s *Service) CreateDockerContainer(name string, config *edgeModel.DockerContainerCreateConfig) (*edgeModel.DockerContainerCreateResult, error) {
	containerName := strings.TrimSpace(name)
	if containerName == "" {
		return nil, fmt.Errorf("container name 不能为空")
	}
	if config == nil {
		return nil, fmt.Errorf("container config 不能为空")
	}
	bodyBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode docker container config failed: %w", err)
	}

	path := "/containers/create?name=" + url.QueryEscape(containerName)
	result := new(edgeModel.DockerContainerCreateResult)
	if err = s.fetchJSON(currentDockerAPIURL(), http.MethodPost, path, bytes.NewReader(bodyBytes), result); err != nil {
		return nil, fmt.Errorf("docker api create container failed: %w", err)
	}
	return result, nil
}

// PullDockerImage 拉取本机 Docker 镜像。
//
// 设计来源：
// - 边缘服务后续要按本机架构准备浏览器镜像；
// - 拉取动作必须发生在边缘节点本机 Docker 上；
// - 这里只调用 Docker Engine API，不保存数据库，也不决定中心服务端的镜像策略。
//
// 维护边界：
// - image/tag 参数在这里转换为 Docker `/images/create` 的 fromImage/tag；
// - 返回 Docker 流式事件，方便前端或中心端展示进度；
// - 不在这里写死业务镜像名，后续镜像选择规则应由中心服务端或配置下发。
func (s *Service) PullDockerImage(param *edgeModel.PullImageRequest) ([]edgeModel.DockerPullEvent, error) {
	return s.PullDockerImageWithProgress(param, nil)
}

// PullDockerImageWithProgress 拉取镜像并把 Docker JSON Lines 事件同步回调给调用方。
//
// 设计来源：
// - 用户指出 Docker pull 容易超过普通 HTTP 超时，需要通过 SSE 观察真实进度；
// - Docker Engine 本身已经返回逐层事件，因此 Service 层保留回调入口，HTTP 层只负责把事件转成任务进度。
//
// 职责边界：
// - 这里仍然只访问本机 Docker API，不保存任务、不知道 SSE；
// - onEvent 失败不会影响 Docker 拉取，避免展示层错误中断底层镜像准备。
func (s *Service) PullDockerImageWithProgress(param *edgeModel.PullImageRequest, onEvent func(edgeModel.DockerPullEvent)) ([]edgeModel.DockerPullEvent, error) {
	if param == nil {
		return nil, fmt.Errorf("请求参数不能为空")
	}
	image, tag := splitImageReference(param.Image, param.Tag)
	if image == "" {
		return nil, fmt.Errorf("image 不能为空")
	}

	query := url.Values{}
	query.Set("fromImage", image)
	if tag != "" {
		query.Set("tag", tag)
	}

	var events []edgeModel.DockerPullEvent
	if err := s.fetchDockerStream(http.MethodPost, "/images/create?"+query.Encode(), nil, func(line []byte) error {
		event := edgeModel.DockerPullEvent{}
		if err := json.Unmarshal(line, &event); err != nil {
			return err
		}
		events = append(events, event)
		if onEvent != nil {
			onEvent(event)
		}
		if event.Error != "" {
			return fmt.Errorf("%s", event.Error)
		}
		return nil
	}); err != nil {
		return events, fmt.Errorf("docker api pull image failed: %w", err)
	}
	return events, nil
}

// RemoveDockerImage 删除本机 Docker 镜像。
//
// 这个动作会改变本机 Docker 状态，所以 API 明确放在 edge 域内。
// 调用方必须传 image，Service 只负责转发到 Docker API，并把 Docker 返回的删除/解绑结果结构化。
func (s *Service) RemoveDockerImage(param *edgeModel.RemoveImageRequest) ([]edgeModel.DockerImageRemoveResult, error) {
	if param == nil {
		return nil, fmt.Errorf("请求参数不能为空")
	}
	image := strings.TrimSpace(param.Image)
	if image == "" {
		return nil, fmt.Errorf("image 不能为空")
	}

	query := url.Values{}
	if param.Force {
		query.Set("force", "1")
	}
	if param.NoPrune {
		query.Set("noprune", "1")
	}

	path := "/images/" + url.PathEscape(image)
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var result []edgeModel.DockerImageRemoveResult
	if err := s.fetchJSON(currentDockerAPIURL(), http.MethodDelete, path, nil, &result); err != nil {
		return nil, fmt.Errorf("docker api remove image failed: %w", err)
	}
	return result, nil
}

// StartDockerContainer 启动本机 Docker 容器。
//
// 设计来源：
// - 边缘服务接下来要管理本机浏览器实例，容器 start 是最小生命周期动作；
// - 这里直接调用 Docker Engine API，不依赖中心 nodeId，也不写数据库；
// - Docker 返回 304 时说明容器已经处于运行态，统一转成 not-modified 响应，方便前端展示。
func (s *Service) StartDockerContainer(containerID string) (*edgeModel.ContainerActionResult, error) {
	return s.executeContainerAction(containerID, "start", nil)
}

// StopDockerContainer 停止本机 Docker 容器。
//
// timeoutSeconds 对应 Docker stop 的 `t` 参数；不传时默认等待 10 秒，避免立刻强停导致浏览器登录态或缓存写入中断。
// 这个方法只负责本机容器动作，不判断容器是否属于某个用户或中心节点。
func (s *Service) StopDockerContainer(containerID string, param *edgeModel.ContainerActionRequest) (*edgeModel.ContainerActionResult, error) {
	timeoutSeconds, err := normalizeContainerTimeout(param, 10)
	if err != nil {
		return nil, err
	}
	return s.executeContainerAction(containerID, "stop", &timeoutSeconds)
}

// RestartDockerContainer 重启本机 Docker 容器。
//
// restart 常用于浏览器实例配置变更后的本机重载。这里仍然只处理 Docker 动作，
// 后续 profile、compose、实例状态的业务编排应在更上层的实例 Service 中完成。
func (s *Service) RestartDockerContainer(containerID string, param *edgeModel.ContainerActionRequest) (*edgeModel.ContainerActionResult, error) {
	timeoutSeconds, err := normalizeContainerTimeout(param, 10)
	if err != nil {
		return nil, err
	}
	return s.executeContainerAction(containerID, "restart", &timeoutSeconds)
}

// RemoveDockerContainer 删除本机 Docker 容器。
//
// 这个方法目前只供 BrowserEnv.run 的 forceRecreate 使用：
// - 它允许重建容器，但不删除环境包目录和 browser-data/profile；
// - force 参数由上层显式传入，避免底层偷偷强删。
func (s *Service) RemoveDockerContainer(containerID string, force bool) error {
	id := strings.TrimSpace(containerID)
	if id == "" {
		return fmt.Errorf("container id 不能为空")
	}
	query := url.Values{}
	if force {
		query.Set("force", "1")
	}
	path := "/containers/" + url.PathEscape(id)
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if _, err := s.fetchDockerAction(http.MethodDelete, path, nil); err != nil {
		return fmt.Errorf("docker api remove container failed: %w", err)
	}
	return nil
}

// ExecDockerContainer 在本机 Docker 容器内执行受控命令。
//
// 设计来源：
// - timezone / 出口 IP 必须在浏览器容器内探测，Go 边缘服务宿主机直连会得到错误出口；
// - 当前方法只给内部 Service 使用，不注册 HTTP 路由，不向前端开放任意命令执行能力；
// - Tty=true 是为了让 Docker API 返回普通 stdout/stderr，避免处理 multiplexed stream。
func (s *Service) ExecDockerContainer(containerID string, cmd []string) (*edgeModel.DockerContainerExecResult, error) {
	id := strings.TrimSpace(containerID)
	if id == "" {
		return nil, fmt.Errorf("container id 不能为空")
	}
	if len(cmd) == 0 {
		return nil, fmt.Errorf("exec cmd 不能为空")
	}
	createConfig := edgeModel.DockerContainerExecCreateConfig{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          cmd,
	}
	createBody, err := json.Marshal(createConfig)
	if err != nil {
		return nil, err
	}
	createResult := new(edgeModel.DockerContainerExecCreateResult)
	if err = s.fetchDockerJSON(http.MethodPost, "/containers/"+url.PathEscape(id)+"/exec", bytes.NewReader(createBody), createResult); err != nil {
		return nil, fmt.Errorf("docker api exec create failed: %w", err)
	}
	if strings.TrimSpace(createResult.ID) == "" {
		return nil, fmt.Errorf("docker api exec id 为空")
	}

	startConfig := edgeModel.DockerContainerExecStartConfig{Detach: false, Tty: true}
	startBody, err := json.Marshal(startConfig)
	if err != nil {
		return nil, err
	}
	outputBytes, err := s.fetchDockerRaw(http.MethodPost, "/exec/"+url.PathEscape(createResult.ID)+"/start", bytes.NewReader(startBody))
	if err != nil {
		return nil, fmt.Errorf("docker api exec start failed: %w", err)
	}

	inspect := new(edgeModel.DockerContainerExecInspectResult)
	if err = s.fetchDockerJSON(http.MethodGet, "/exec/"+url.PathEscape(createResult.ID)+"/json", nil, inspect); err != nil {
		return nil, fmt.Errorf("docker api exec inspect failed: %w", err)
	}
	exitCode := 0
	if inspect.ExitCode != nil {
		exitCode = *inspect.ExitCode
	}
	return &edgeModel.DockerContainerExecResult{
		ExecID:   createResult.ID,
		Output:   string(outputBytes),
		ExitCode: exitCode,
	}, nil
}

// probeDockerEngine 是边缘服务访问 Docker Engine API 的统一入口。
//
// 设计来源：
// - 之前的 nodes/probe-docker 混入了“节点中控”的语义；
// - 现在 Client 作为边缘服务，只需要读取本机 Docker 2375；
// - 因此所有本机 Docker 基础信息都从这里收口，后续扩展 socket/TLS 时也只改这一处和配置。
func (s *Service) probeDockerEngine() (*edgeModel.DeviceInfo, error) {
	dockerAPIURL := normalizeDockerAPIURL(Settings.Conf.DockerConfig.APIURL)
	if dockerAPIURL == "" {
		return nil, fmt.Errorf("docker api url 不能为空")
	}
	deviceIP := extractURLHost(dockerAPIURL)

	if err := s.fetchJSON(dockerAPIURL, http.MethodGet, "/_ping", nil, nil); err != nil {
		return nil, fmt.Errorf("docker api ping failed: %w", err)
	}

	info := new(edgeModel.DockerEngineInfoResponse)
	if err := s.fetchJSON(dockerAPIURL, http.MethodGet, "/info", nil, info); err != nil {
		return nil, fmt.Errorf("docker api info failed: %w", err)
	}

	version := new(edgeModel.DockerEngineVersionResponse)
	if err := s.fetchJSON(dockerAPIURL, http.MethodGet, "/version", nil, version); err != nil {
		return nil, fmt.Errorf("docker api version failed: %w", err)
	}

	rawArch := firstNonEmpty(version.Arch, info.Architecture)
	return &edgeModel.DeviceInfo{
		DeviceIP:            deviceIP,
		DockerAPIURL:        dockerAPIURL,
		DeviceOS:            firstNonEmpty(info.OperatingSystem, version.OS, info.OSType),
		DeviceArch:          normalizeDeviceArch(rawArch),
		DeviceRawArch:       rawArch,
		CPUCores:            info.NCPU,
		MemoryTotalBytes:    info.MemTotal,
		DockerVersion:       firstNonEmpty(version.Version, info.ServerVersion),
		DockerAPIVersion:    strings.TrimSpace(version.APIVersion),
		ComposeVersion:      "",
		LastDockerStatus:    "available",
		LastDockerMessage:   "ok",
		LastImagesCount:     info.Images,
		LastContainersCount: info.Containers,
		CheckedAt:           time.Now().Unix(),
	}, nil
}

// normalizeDockerAPIURL 规范化 Docker Engine API 根地址。
//
// 允许配置写 `127.0.0.1:2375` 或 `http://127.0.0.1:2375`；
// 如果没有 scheme，默认补 `http://`，因为当前边缘服务默认使用 Docker 2375 明文 HTTP。
func normalizeDockerAPIURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	return strings.TrimRight(value, "/")
}

// extractURLHost 从 Docker API URL 中提取主机地址。
//
// 这个字段用于返回 deviceIp，帮助中心服务端或前端知道当前边缘服务正在读取哪个 Docker API。
func extractURLHost(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
}

// normalizeDeviceArch 把 Docker 返回的原始架构归一化。
//
// 后续镜像选择只允许依赖 amd64 / arm64 / unknown 三类稳定值，不要在业务逻辑里散写 x86_64、aarch64 等判断。
func normalizeDeviceArch(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "amd64", "x86-64", "x64":
		return edgeModel.ArchAMD64
	case "arm64", "aarch64", "armv8", "arm-v8":
		return edgeModel.ArchARM64
	default:
		return edgeModel.ArchUnknown
	}
}

// splitImageReference 把调用方传入的镜像引用拆成 Docker API 需要的 image 和 tag。
//
// Docker 镜像名里可能包含 registry 端口，例如 `127.0.0.1:5000/ns/app:tag`。
// 因此不能简单按第一个冒号拆分，只能在最后一个 `/` 之后再判断 tag 冒号。
func splitImageReference(rawImage string, rawTag string) (string, string) {
	image := strings.TrimSpace(rawImage)
	tag := strings.TrimSpace(rawTag)
	if image == "" || tag != "" {
		return image, tag
	}

	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		return image[:lastColon], image[lastColon+1:]
	}
	return image, tag
}

// firstNonEmpty 返回第一个非空字符串。
//
// Docker `/info` 和 `/version` 会提供部分重叠字段，这里集中处理优先级，避免调用处反复写 trim 判断。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// applyProjectContainerMetadata 判断容器是否属于当前项目，并补充项目角色。
//
// 设计来源：
// - `/edge/docker/containers` 的使用场景是边缘控制台，不是通用 Docker 管理器；
// - 用户明确要求容器列表只看本项目相关容器，并区分边缘服务自身和浏览器容器服务；
// - 早期浏览器容器已经带 `bv.envId`，新容器会额外带 `bv.project/bv.role`，这里同时兼容两种识别方式。
//
// 返回 false 表示该容器是其他项目容器，不能出现在边缘服务容器列表里。
func applyProjectContainerMetadata(container *edgeModel.DockerContainer) bool {
	if container == nil {
		return false
	}
	labels := container.Labels
	if labels == nil {
		labels = map[string]string{}
		container.Labels = labels
	}

	if isBrowserEnvContainer(container) {
		container.ProjectRole = "browser-env"
		container.EnvID = labels["bv.envId"]
		container.UserID = labels["bv.userId"]
		container.RPAType = labels["bv.rpaType"]
		return true
	}
	if isEdgeServiceContainer(container) {
		container.ProjectRole = "edge-service"
		return true
	}
	return false
}

// isBrowserEnvContainer 识别由本项目创建的浏览器环境容器。
//
// 新容器使用 `bv.project=private-browser-client` + `bv.role=browser-env`；
// 为了让历史旧容器不丢失，也兼容 `bv.envId` 和 `bv-` 名称前缀。
func isBrowserEnvContainer(container *edgeModel.DockerContainer) bool {
	labels := container.Labels
	if labels["bv.project"] == "private-browser-client" && labels["bv.role"] == "browser-env" {
		return true
	}
	if strings.TrimSpace(labels["bv.envId"]) != "" {
		return true
	}
	for _, name := range container.Names {
		if strings.HasPrefix(strings.TrimPrefix(strings.TrimSpace(name), "/"), "bv-") {
			return true
		}
	}
	return false
}

// isEdgeServiceContainer 识别 Private_Browser_Client 边缘服务自身容器。
//
// 本地 `go run` 时服务不在 Docker 里，所以列表可能只出现 browser-env；
// Docker 部署时推荐给容器加 `bv.project=private-browser-client,bv.role=edge-service`，
// 同时这里也按容器名和镜像名兜底识别，方便 docker run/compose 不同部署方式。
func isEdgeServiceContainer(container *edgeModel.DockerContainer) bool {
	labels := container.Labels
	if labels["bv.project"] == "private-browser-client" && labels["bv.role"] == "edge-service" {
		return true
	}
	for _, name := range container.Names {
		normalized := strings.TrimPrefix(strings.TrimSpace(name), "/")
		if normalized == "private-browser-client" || strings.HasPrefix(normalized, "private-browser-client-") {
			return true
		}
	}
	image := strings.ToLower(strings.TrimSpace(container.Image))
	return strings.Contains(image, "private-browser-client")
}

// normalizeStringSlice 把 Docker 可能返回的 nil 切片统一成空数组。
//
// 这能让前端和中心服务端少处理一种 null 情况，尤其是 dangling 镜像经常没有 RepoTags。
func normalizeStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// normalizeStringMap 把 Docker 可能返回的 nil labels 统一成空对象。
//
// labels 后续会用于识别哪些容器由边缘服务创建，保持稳定对象结构能减少调用方判断。
func normalizeStringMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}

// buildContainerPorts 把 Docker 原始端口模型转换为边缘服务对外模型。
//
// 这个转换单独存在，是为了把 Docker 首字母大写字段隔离在内部解码模型里，对外 API 统一小驼峰。
func buildContainerPorts(rawPorts []edgeModel.DockerEngineContainerPortResponse) []edgeModel.DockerContainerPort {
	if rawPorts == nil {
		return []edgeModel.DockerContainerPort{}
	}
	ports := make([]edgeModel.DockerContainerPort, 0, len(rawPorts))
	for _, raw := range rawPorts {
		ports = append(ports, edgeModel.DockerContainerPort{
			IP:          raw.IP,
			PrivatePort: raw.PrivatePort,
			PublicPort:  raw.PublicPort,
			Type:        raw.Type,
		})
	}
	return ports
}

// currentDockerAPIURL 返回当前配置中的 Docker API 地址。
//
// 它把 Settings 读取和 URL 规范化集中在一处，避免新增 Docker API 方法时散写配置读取逻辑。
func currentDockerAPIURL() string {
	return normalizeDockerAPIURL(Settings.Conf.DockerConfig.APIURL)
}

// normalizeContainerTimeout 归一化容器停止/重启等待时间。
//
// 这个函数单独存在，是因为 timeout 会影响容器优雅退出和浏览器数据落盘。
// 默认值选择 10 秒：既不会长期阻塞 API，也给浏览器进程留出基础清理时间。
func normalizeContainerTimeout(param *edgeModel.ContainerActionRequest, defaultSeconds int) (int, error) {
	if param == nil || param.TimeoutSeconds == nil {
		return defaultSeconds, nil
	}
	timeoutSeconds := *param.TimeoutSeconds
	if timeoutSeconds < 0 {
		return 0, fmt.Errorf("timeoutSeconds 不能小于 0")
	}
	if timeoutSeconds > 3600 {
		return 0, fmt.Errorf("timeoutSeconds 不能大于 3600")
	}
	return timeoutSeconds, nil
}

// executeContainerAction 统一执行 Docker 容器生命周期动作。
//
// 维护边界：
// - action 只允许内部固定传入 start / stop / restart，不接收外部任意字符串；
// - 这里负责拼接 Docker API 路径、处理 204/304 和错误响应；
// - 不在这里做浏览器实例业务校验，避免底层 Docker 管理能力和未来实例编排混在一起。
func (s *Service) executeContainerAction(containerID string, action string, timeoutSeconds *int) (*edgeModel.ContainerActionResult, error) {
	id := strings.TrimSpace(containerID)
	if id == "" {
		return nil, fmt.Errorf("container id 不能为空")
	}

	path := "/containers/" + url.PathEscape(id) + "/" + action
	if timeoutSeconds != nil {
		query := url.Values{}
		query.Set("t", fmt.Sprintf("%d", *timeoutSeconds))
		path += "?" + query.Encode()
	}

	statusCode, err := s.fetchDockerAction(http.MethodPost, path, nil)
	if err != nil {
		return nil, fmt.Errorf("docker api %s container failed: %w", action, err)
	}

	status := "success"
	message := "ok"
	if statusCode == http.StatusNotModified {
		status = "not-modified"
		message = "container already in target state"
	}

	return &edgeModel.ContainerActionResult{
		ContainerID: id,
		Action:      action,
		Status:      status,
		Message:     message,
		CheckedAt:   time.Now().Unix(),
	}, nil
}

// fetchDockerAction 封装 Docker 变更类接口调用。
//
// 与 fetchJSON 不同，start / stop / restart 这类接口成功时通常没有响应体，只靠 HTTP 状态码表达结果。
// 这里保留 304 作为可识别结果，404/500 等错误仍然交给 buildDockerAPIError 统一整理。
func (s *Service) fetchDockerAction(method, path string, body io.Reader) (int, error) {
	dockerAPIURL := currentDockerAPIURL()
	if dockerAPIURL == "" {
		return 0, fmt.Errorf("docker api url 不能为空")
	}

	endpoint := fmt.Sprintf("%s%s", strings.TrimRight(dockerAPIURL, "/"), path)
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request docker api failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return resp.StatusCode, buildDockerAPIError(resp.StatusCode, bodyBytes)
	}
	return resp.StatusCode, nil
}

// fetchDockerJSON 使用当前 Docker API 地址请求并解析 JSON。
//
// 与 fetchJSON 的区别是这里自动读取 Settings 中的本机 Docker API，供 Docker exec 等内部方法复用。
func (s *Service) fetchDockerJSON(method, path string, body io.Reader, target any) error {
	dockerAPIURL := currentDockerAPIURL()
	if dockerAPIURL == "" {
		return fmt.Errorf("docker api url 不能为空")
	}
	return s.fetchJSON(dockerAPIURL, method, path, body, target)
}

// fetchDockerRaw 读取 Docker API 原始响应体。
//
// Docker exec start 返回的是命令输出流，不是 JSON；这里只做 HTTP 状态和错误处理，不解释内容。
func (s *Service) fetchDockerRaw(method, path string, body io.Reader) ([]byte, error) {
	dockerAPIURL := currentDockerAPIURL()
	if dockerAPIURL == "" {
		return nil, fmt.Errorf("docker api url 不能为空")
	}
	endpoint := fmt.Sprintf("%s%s", strings.TrimRight(dockerAPIURL, "/"), path)
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request docker api failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, buildDockerAPIError(resp.StatusCode, bodyBytes)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
}

// fetchJSON 封装访问 Docker Engine API 的公共逻辑。
//
// 它统一处理请求创建、HTTP 状态码和 JSON 解码。对于 `/_ping` 这类纯文本成功响应，target 传 nil 即可。
func (s *Service) fetchJSON(baseURL, method, path string, body io.Reader, target any) error {
	endpoint := fmt.Sprintf("%s%s", strings.TrimRight(baseURL, "/"), path)
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request docker api failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return buildDockerAPIError(resp.StatusCode, bodyBytes)
	}
	if target == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode docker api response failed: %w", err)
	}
	return nil
}

// fetchDockerStream 处理 Docker Engine 返回的 JSON Lines 流式响应。
//
// Docker pull 这类接口不会返回普通 JSON 对象，而是一行一行输出状态事件。
// 这里把流式读取收口，避免 pull-image 以外的后续流式接口重复写扫描逻辑。
func (s *Service) fetchDockerStream(method, path string, body io.Reader, handleLine func([]byte) error) error {
	dockerAPIURL := currentDockerAPIURL()
	if dockerAPIURL == "" {
		return fmt.Errorf("docker api url 不能为空")
	}

	endpoint := fmt.Sprintf("%s%s", strings.TrimRight(dockerAPIURL, "/"), path)
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request docker api failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return buildDockerAPIError(resp.StatusCode, bodyBytes)
	}

	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := handleLine([]byte(line)); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read docker api stream failed: %w", err)
	}
	return nil
}

// buildDockerAPIError 把 Docker API 错误响应整理成人能读懂的错误。
//
// Docker 失败时通常返回 `{"message":"..."}`，之前直接把完整 body 拼出去，Apifox 里会显得很乱。
// 这里保留 HTTP 状态码，同时优先提取 message；如果 Docker 返回的不是 JSON，再回退到原始 body。
func buildDockerAPIError(statusCode int, bodyBytes []byte) error {
	body := strings.TrimSpace(string(bodyBytes))
	if body == "" {
		return fmt.Errorf("docker api status=%d", statusCode)
	}

	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
		return fmt.Errorf("docker api status=%d, message=%s", statusCode, strings.TrimSpace(payload.Message))
	}

	return fmt.Errorf("docker api status=%d, body=%s", statusCode, body)
}
