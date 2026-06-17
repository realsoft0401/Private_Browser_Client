package SlotRuntime

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	edgeModel "private_browser_client/Models/Edge"
	slotModel "private_browser_client/Models/Slot"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

// Initializer 定义 slot 资源位容器生命周期最小边界。
//
// 当前新模型里 slot 是常驻运行资源，所以 create-slot / destroy-slot / reinit-slot
// 都应该通过这层去管理本机容器，而不是在 Slot Service 里散写 Docker 细节。
type Initializer interface {
	Initialize(slot *slotModel.Slot) error
	Destroy(slot *slotModel.Slot) error
	Reinitialize(slot *slotModel.Slot) error
}

var initializer Initializer = NewDockerInitializer()

func GetInitializer() Initializer {
	return initializer
}

func SetInitializer(value Initializer) {
	if value == nil {
		initializer = NewDockerInitializer()
		return
	}
	initializer = value
}

type DockerInitializer struct{}

func NewDockerInitializer() *DockerInitializer {
	return &DockerInitializer{}
}

// Initialize 在 create-slot 成功后立即准备本机常驻运行容器。
//
// 设计来源：
// - 你已经明确 slot 创建后就应该把对应资源初始化出来；
// - 但当前项目还没接 package 加载和正式浏览器运行镜像，因此这里先准备一个长期存活的运行容器；
// - 后续真正的浏览器镜像策略和 package 加载流程，可以继续在这层扩展，不要破坏 Slot Service 主链路。
func (d *DockerInitializer) Initialize(slot *slotModel.Slot) error {
	if slot == nil {
		return fmt.Errorf("slot 不能为空")
	}
	config, ok := currentRuntimeConfig()
	if !ok || !config.Enabled {
		return nil
	}

	containerName := buildSlotContainerName(slot.SlotID, config.ContainerNamePrefix)
	cdpPort, vncPort, err := allocateSlotPorts(slot.SlotID, config)
	if err != nil {
		return err
	}
	slot.CDPPort = optionalInt(cdpPort)
	slot.VNCPort = optionalInt(vncPort)

	cdpKey := dockerPortKey(config.InternalCDPPort)
	vncKey := dockerPortKey(config.InternalVNCPort)
	createConfig := &edgeModel.DockerContainerCreateConfig{
		Image: config.Image,
		Cmd:   cloneStringSlice(config.Command),
		Env: []string{
			"DEBUG_PORT=" + strconv.Itoa(config.InternalCDPPort),
			"ENABLE_VNC=" + strconv.FormatBool(config.EnableVNC),
			"VNC_PORT=" + strconv.Itoa(config.InternalVNCPort),
		},
		Labels: map[string]string{
			"bv.project": "private-browser-client",
			"bv.role":    "slot-runtime",
			"bv.slotId":  slot.SlotID,
		},
		ExposedPorts: map[string]struct{}{
			cdpKey: {},
			vncKey: {},
		},
		HostConfig: edgeModel.DockerContainerHostConfig{
			RestartPolicy: edgeModel.DockerContainerRestartPolicy{Name: "unless-stopped"},
			PortBindings: map[string][]edgeModel.DockerPortBinding{
				cdpKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(cdpPort)}},
				vncKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(vncPort)}},
			},
		},
	}

	edge := edgeService.NewEdgeService()
	createResult, err := edge.CreateDockerContainer(containerName, createConfig)
	if err != nil {
		if config.AutoPullMissingImage && shouldPullImageBeforeRetry(err) {
			if _, pullErr := edge.PullDockerImage(config.Image); pullErr != nil {
				return fmt.Errorf("slot runtime pull image failed: %w", pullErr)
			}
			createResult, err = edge.CreateDockerContainer(containerName, createConfig)
		}
	}
	if err != nil {
		return fmt.Errorf("slot runtime create container failed: %w", err)
	}
	if _, err = edge.StartDockerContainer(createResult.ID); err != nil {
		_ = edge.RemoveDockerContainer(createResult.ID, true)
		return fmt.Errorf("slot runtime start container failed: %w", err)
	}

	slot.ContainerID = optionalString(createResult.ID)
	slot.ContainerName = optionalString(containerName)
	slot.RuntimeImage = optionalString(config.Image)
	slot.ContainerStatus = optionalString("running")
	return nil
}

func (d *DockerInitializer) Destroy(slot *slotModel.Slot) error {
	if slot == nil || slot.ContainerID == nil || strings.TrimSpace(*slot.ContainerID) == "" {
		return nil
	}
	if !hasRuntimeConfig() {
		return nil
	}

	edge := edgeService.NewEdgeService()
	if err := edge.RemoveDockerContainer(*slot.ContainerID, true); err != nil {
		return fmt.Errorf("slot runtime remove container failed: %w", err)
	}
	slot.ContainerStatus = optionalString("removed")
	return nil
}

func (d *DockerInitializer) Reinitialize(slot *slotModel.Slot) error {
	if err := d.Destroy(slot); err != nil {
		return err
	}

	slot.ContainerID = nil
	slot.ContainerName = nil
	slot.RuntimeImage = nil
	slot.ContainerStatus = nil
	slot.CDPPort = nil
	slot.VNCPort = nil
	return d.Initialize(slot)
}

func buildSlotContainerName(slotID string, prefix string) string {
	return strings.TrimSpace(prefix) + "-" + strings.TrimSpace(slotID)
}

func currentRuntimeConfig() (*Settings.SlotRuntimeConfig, bool) {
	if Settings.Conf == nil || Settings.Conf.ConfigFile == "" {
		return nil, false
	}
	if Settings.Conf.SlotRuntimeConfig == nil {
		return nil, false
	}
	return Settings.Conf.SlotRuntimeConfig, true
}

func hasRuntimeConfig() bool {
	_, ok := currentRuntimeConfig()
	return ok
}

func shouldPullImageBeforeRetry(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such image") || strings.Contains(message, "pull access denied")
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func cloneStringSlice(value []string) []string {
	if len(value) == 0 {
		return nil
	}
	cloned := make([]string, len(value))
	copy(cloned, value)
	return cloned
}

func allocateSlotPorts(slotID string, config *Settings.SlotRuntimeConfig) (int, int, error) {
	if config == nil {
		return 0, 0, fmt.Errorf("slot runtime config 不能为空")
	}

	if slotIndex, ok := parseSlotIndex(slotID); ok {
		preferredCDP := config.HostCDPBasePort + slotIndex
		preferredVNC := config.HostVNCBasePort + slotIndex
		if isTCPPortAvailable(preferredCDP) && isTCPPortAvailable(preferredVNC) {
			return preferredCDP, preferredVNC, nil
		}
	}

	cdpPort, err := findAvailableTCPPort(config.HostCDPBasePort)
	if err != nil {
		return 0, 0, err
	}
	vncPort, err := findAvailableTCPPort(config.HostVNCBasePort)
	if err != nil {
		return 0, 0, err
	}
	return cdpPort, vncPort, nil
}

func parseSlotIndex(slotID string) (int, bool) {
	trimmed := strings.TrimSpace(slotID)
	if trimmed == "" {
		return 0, false
	}
	for index := len(trimmed) - 1; index >= 0; index-- {
		if trimmed[index] < '0' || trimmed[index] > '9' {
			if index == len(trimmed)-1 {
				return 0, false
			}
			value, err := strconv.Atoi(trimmed[index+1:])
			return value, err == nil && value >= 0
		}
	}
	value, err := strconv.Atoi(trimmed)
	return value, err == nil && value >= 0
}

func findAvailableTCPPort(start int) (int, error) {
	for port := start; port < start+2000; port++ {
		if isTCPPortAvailable(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("从端口 %d 开始未找到可用 TCP 端口", start)
}

func isTCPPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func dockerPortKey(port int) string {
	return strconv.Itoa(port) + "/tcp"
}

func optionalInt(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}
