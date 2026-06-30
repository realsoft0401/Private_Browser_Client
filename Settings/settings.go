package Settings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

var (
	// ConfigFileName 保留为旧测试/兼容常量，但不再是 Client 启动必需项。
	//
	// 这次改造后，Client 的正式启动方式是“默认值 + 环境变量覆盖”。
	// 如果项目目录里恰好还有这个文件，Settings.Init 仍然可以读取；
	// 但缺少它不应再阻塞启动。
	ConfigFileName = "config-docker.yaml"

	// Conf 持有当前服务进程的全局配置。
	//
	// old 架构里，Routes、Health、Discovery、Edge 都是从全局配置读取运行参数。
	// 新项目继续沿用这条总线，避免把配置对象层层手传，破坏既有架构阅读方式。
	Conf = new(AppConfig)

	configEngine = viper.New()
)

// AppConfig 是新 Client 的全局配置模型。
//
// 这次先把 README 里已经收口的边界直接落成配置域：
// - server：本机 HTTP 服务监听参数
// - docker：本机 Docker API 入口
// - discovery：UDP 自动发现广播配置
// - swagger：Swagger/OpenAPI 工具页配置
//
// ProjectRoot / ConfigFile / Env 是运行时补进去的元信息，
// 它们主要服务于健康检查、排障和路由静态文件查找，不从 YAML 反序列化。
type AppConfig struct {
	Name    string `mapstructure:"name"`
	Mode    string `mapstructure:"mode"`
	Version string `mapstructure:"version"`

	ProjectRoot string `mapstructure:"-"`
	ConfigFile  string `mapstructure:"-"`
	Env         string `mapstructure:"-"`

	*ServerConfig       `mapstructure:"server"`
	*DockerConfig       `mapstructure:"docker"`
	*DiscoveryConfig    `mapstructure:"discovery"`
	*HeartbeatConfig    `mapstructure:"heartbeat"`
	*NodeRegisterConfig `mapstructure:"node_register"`
	*SwaggerConfig      `mapstructure:"swagger"`
	*SlotRuntimeConfig  `mapstructure:"slot_runtime"`
}

// ServerConfig 描述 Client 本机 HTTP 服务入口。
type ServerConfig struct {
	Host                string `mapstructure:"host"`
	Port                int    `mapstructure:"port"`
	ReadTimeoutSeconds  int    `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `mapstructure:"write_timeout_seconds"`
}

// DockerConfig 描述 Client 访问本机 Docker Engine 的方式。
type DockerConfig struct {
	APIURL string `mapstructure:"api_url"`
}

// DiscoveryConfig 描述 Client 在独立内网中的 UDP 自动发现广播。
type DiscoveryConfig struct {
	Enabled          bool   `mapstructure:"enabled"`
	BroadcastAddress string `mapstructure:"broadcast_address"`
	Port             int    `mapstructure:"port"`
	IntervalSeconds  int    `mapstructure:"interval_seconds"`
	Magic            string `mapstructure:"magic"`
	ProtocolVersion  int    `mapstructure:"protocol_version"`
	Group            string `mapstructure:"group"`
	AdvertiseHost    string `mapstructure:"advertise_host"`
	AdvertiseBaseURL string `mapstructure:"advertise_base_url"`
}

// HeartbeatConfig 描述 Client -> Node Server 的 HTTP 活性心跳配置。
//
// 设计来源：
// - 当前正式口径已经收紧为“UDP 只负责发现，heartbeat 只负责证明服务仍然存在”；
// - 因此这组配置必须继续保留，而且默认应可用，但绝不能再被理解成 discovery 入口；
// - Node 侧收到 heartbeat 后只能更新已知节点的活性摘要，不能凭 heartbeat 新建 discovered 或正式绑定。
//
// 维护约束：
// - `ServerBaseURL` 只表示 Node Server 的基地址，不包含 path；
// - 这组配置负责服务存在性回执，不负责 clientId 分配，不负责 bind；
// - 跨机部署时必须填写真实 Node 内网地址，不能把 `127.0.0.1:3400` 当成跨机器通用模板。
type HeartbeatConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	ServerBaseURL   string `mapstructure:"server_base_url"`
	IntervalSeconds int    `mapstructure:"interval_seconds"`
}

// NodeRegisterConfig 描述 Client 与 Node Server 的中心登记协同参数。
//
// 设计来源：
// - 最新正式链路已经收口为“Node 发起 bind，Client 接收 assign，并本地留存 node-registration.json”；
// - node_register 不参与 discovery，但仍是中心唯一设备身份写入 Client 的正式能力；
// - 中心身份仍由 Node Server 生成，Client 不自行发号。
type NodeRegisterConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	ServerBaseURL string `mapstructure:"server_base_url"`
	MainAccountID string `mapstructure:"main_account_id"`
	NodeName      string `mapstructure:"node_name"`
	EdgeAPIKey    string `mapstructure:"edge_api_key"`
}

// SwaggerConfig 描述 Swagger / OpenAPI 工具页能力。
type SwaggerConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// SlotRuntimeConfig 描述 slot 资源位创建后要初始化的本机运行容器。
//
// 设计来源：
// - 新模型已经明确“包是包，容器是容器”，slot 创建完成后应先有常驻运行资源；
// - 这层配置只描述 Client 本机如何准备 slot 对应的运行容器，不描述平台配额，不描述 package 资产；
// - 当前默认镜像使用一个可长期运行的轻量占位镜像，后续浏览器正式运行镜像仍应由更上层下发或平台决定。
type SlotRuntimeConfig struct {
	Enabled              bool     `mapstructure:"enabled"`
	Image                string   `mapstructure:"image"`
	Command              []string `mapstructure:"command"`
	ContainerNamePrefix  string   `mapstructure:"container_name_prefix"`
	AutoPullMissingImage bool     `mapstructure:"auto_pull_missing_image"`
	EnableVNC            bool     `mapstructure:"enable_vnc"`
	InternalCDPPort      int      `mapstructure:"internal_cdp_port"`
	InternalVNCPort      int      `mapstructure:"internal_vnc_port"`
	HostCDPBasePort      int      `mapstructure:"host_cdp_base_port"`
	HostVNCBasePort      int      `mapstructure:"host_vnc_base_port"`
}

// Init 负责加载当前部署配置。
//
// 这里继续沿用 old 架构的配置入口方式：
// main -> Infrastructures.Init -> Settings.Init
// 这样后续 Discovery、Health、Routes、Edge 都继续通过 `Settings.Conf` 取值。
func Init(projectRoot string) error {
	configEngine = viper.New()
	configEngine.SetDefault("name", "private-browser-client")
	configEngine.SetDefault("mode", "production")
	configEngine.SetDefault("version", "0.2.0")
	configEngine.SetDefault("server.host", "0.0.0.0")
	configEngine.SetDefault("server.port", 3300)
	configEngine.SetDefault("server.read_timeout_seconds", 15)
	configEngine.SetDefault("server.write_timeout_seconds", 15)
	configEngine.SetDefault("docker.api_url", "http://127.0.0.1:2375")
	configEngine.SetDefault("discovery.enabled", true)
	configEngine.SetDefault("discovery.broadcast_address", "255.255.255.255")
	configEngine.SetDefault("discovery.port", 43000)
	configEngine.SetDefault("discovery.interval_seconds", 15)
	configEngine.SetDefault("discovery.magic", "PRIVATE_BROWSER_CLIENT_DISCOVERY")
	configEngine.SetDefault("discovery.protocol_version", 1)
	configEngine.SetDefault("discovery.group", "default")
	configEngine.SetDefault("heartbeat.enabled", true)
	configEngine.SetDefault("heartbeat.server_base_url", "http://127.0.0.1:3400")
	configEngine.SetDefault("heartbeat.interval_seconds", 15)
	configEngine.SetDefault("node_register.enabled", true)
	configEngine.SetDefault("node_register.server_base_url", "http://127.0.0.1:3400")
	configEngine.SetDefault("node_register.main_account_id", "")
	configEngine.SetDefault("node_register.node_name", "")
	configEngine.SetDefault("node_register.edge_api_key", "private-browser-edge-key")
	configEngine.SetDefault("swagger.enabled", true)
	configEngine.SetDefault("swagger.path", "/swagger")
	configEngine.SetDefault("slot_runtime.enabled", true)
	configEngine.SetDefault("slot_runtime.image", "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64")
	configEngine.SetDefault("slot_runtime.command", []string{})
	configEngine.SetDefault("slot_runtime.container_name_prefix", "private-browser-slot")
	configEngine.SetDefault("slot_runtime.auto_pull_missing_image", true)
	configEngine.SetDefault("slot_runtime.enable_vnc", true)
	configEngine.SetDefault("slot_runtime.internal_cdp_port", 9222)
	configEngine.SetDefault("slot_runtime.internal_vnc_port", 5900)
	configEngine.SetDefault("slot_runtime.host_cdp_base_port", 9200)
	configEngine.SetDefault("slot_runtime.host_vnc_base_port", 9100)

	configEngine.SetEnvPrefix("PRIVATE_BROWSER_CLIENT")
	configEngine.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	configEngine.AutomaticEnv()

	configFile := filepath.Join(projectRoot, "Settings", ConfigFileName)
	if _, err := os.Stat(configFile); err == nil {
		configEngine.SetConfigFile(configFile)
		configEngine.SetConfigType("yaml")
		if err := configEngine.ReadInConfig(); err != nil {
			return fmt.Errorf("read config failed: %w", err)
		}
	}
	if err := configEngine.Unmarshal(Conf); err != nil {
		return fmt.Errorf("unmarshal config failed: %w", err)
	}

	Conf.ProjectRoot = projectRoot
	if _, err := os.Stat(configFile); err == nil {
		Conf.ConfigFile = configFile
	}
	Conf.Env = "docker"
	normalizeRuntimeMode(Conf)
	if Conf.ServerConfig == nil {
		Conf.ServerConfig = &ServerConfig{}
	}
	if Conf.DockerConfig == nil {
		Conf.DockerConfig = &DockerConfig{}
	}
	if Conf.DiscoveryConfig == nil {
		Conf.DiscoveryConfig = &DiscoveryConfig{}
	}
	if Conf.HeartbeatConfig == nil {
		Conf.HeartbeatConfig = &HeartbeatConfig{}
	}
	if Conf.NodeRegisterConfig == nil {
		Conf.NodeRegisterConfig = &NodeRegisterConfig{}
	}
	if Conf.SwaggerConfig == nil {
		Conf.SwaggerConfig = &SwaggerConfig{}
	}
	if Conf.SlotRuntimeConfig == nil {
		Conf.SlotRuntimeConfig = &SlotRuntimeConfig{}
	}
	normalizeServerConfig(Conf.ServerConfig)
	normalizeDiscoveryConfig(Conf.DiscoveryConfig)
	normalizeHeartbeatConfig(Conf.HeartbeatConfig)
	normalizeNodeRegisterConfig(Conf.NodeRegisterConfig)
	normalizeSwaggerConfig(Conf.SwaggerConfig)
	normalizeSlotRuntimeConfig(Conf.SlotRuntimeConfig)

	if configEngine.ConfigFileUsed() != "" {
		configEngine.WatchConfig()
		configEngine.OnConfigChange(func(event fsnotify.Event) {
			updated := new(AppConfig)
			if err := configEngine.Unmarshal(updated); err != nil {
				fmt.Printf("reload config failed, err:%v\n", err)
				return
			}
			updated.ProjectRoot = projectRoot
			updated.ConfigFile = configFile
			updated.Env = "docker"
			normalizeRuntimeMode(updated)
			if updated.ServerConfig == nil {
				updated.ServerConfig = &ServerConfig{}
			}
			if updated.DockerConfig == nil {
				updated.DockerConfig = &DockerConfig{}
			}
			if updated.DiscoveryConfig == nil {
				updated.DiscoveryConfig = &DiscoveryConfig{}
			}
			if updated.HeartbeatConfig == nil {
				updated.HeartbeatConfig = &HeartbeatConfig{}
			}
			if updated.NodeRegisterConfig == nil {
				updated.NodeRegisterConfig = &NodeRegisterConfig{}
			}
			if updated.SwaggerConfig == nil {
				updated.SwaggerConfig = &SwaggerConfig{}
			}
			if updated.SlotRuntimeConfig == nil {
				updated.SlotRuntimeConfig = &SlotRuntimeConfig{}
			}
			normalizeServerConfig(updated.ServerConfig)
			normalizeDiscoveryConfig(updated.DiscoveryConfig)
			normalizeHeartbeatConfig(updated.HeartbeatConfig)
			normalizeNodeRegisterConfig(updated.NodeRegisterConfig)
			normalizeSwaggerConfig(updated.SwaggerConfig)
			normalizeSlotRuntimeConfig(updated.SlotRuntimeConfig)
			Conf = updated
			fmt.Printf("config reloaded: %s\n", event.Name)
		})
	}

	return nil
}

func normalizeRuntimeMode(config *AppConfig) {
	if config == nil {
		return
	}
	config.Mode = "production"
}

func normalizeServerConfig(config *ServerConfig) {
	if config == nil {
		return
	}
	if strings.TrimSpace(config.Host) == "" {
		config.Host = "0.0.0.0"
	}
	if config.Port <= 0 {
		config.Port = 3300
	}
	if config.ReadTimeoutSeconds <= 0 {
		config.ReadTimeoutSeconds = 15
	}
	if config.WriteTimeoutSeconds <= 0 {
		config.WriteTimeoutSeconds = 15
	}
}

func normalizeDiscoveryConfig(config *DiscoveryConfig) {
	if config == nil {
		return
	}
	if strings.TrimSpace(config.BroadcastAddress) == "" {
		config.BroadcastAddress = "255.255.255.255"
	}
	if config.Port <= 0 {
		config.Port = 43000
	}
	if config.IntervalSeconds <= 0 {
		config.IntervalSeconds = 15
	}
	if strings.TrimSpace(config.Magic) == "" {
		config.Magic = "PRIVATE_BROWSER_CLIENT_DISCOVERY"
	}
	if config.ProtocolVersion <= 0 {
		config.ProtocolVersion = 1
	}
	if strings.TrimSpace(config.Group) == "" {
		config.Group = "default"
	}
}

func normalizeHeartbeatConfig(config *HeartbeatConfig) {
	if config == nil {
		return
	}
	if config.IntervalSeconds <= 0 {
		config.IntervalSeconds = 15
	}
	if config.IntervalSeconds < 5 {
		config.IntervalSeconds = 5
	}
	config.ServerBaseURL = strings.TrimRight(strings.TrimSpace(config.ServerBaseURL), "/")
}

func normalizeNodeRegisterConfig(config *NodeRegisterConfig) {
	if config == nil {
		return
	}
	config.ServerBaseURL = strings.TrimRight(strings.TrimSpace(config.ServerBaseURL), "/")
	config.MainAccountID = strings.TrimSpace(config.MainAccountID)
	config.NodeName = strings.TrimSpace(config.NodeName)
	config.EdgeAPIKey = strings.TrimSpace(config.EdgeAPIKey)
	if config.EdgeAPIKey == "" {
		config.EdgeAPIKey = "private-browser-edge-key"
	}
}

func normalizeSwaggerConfig(config *SwaggerConfig) {
	if config == nil {
		return
	}
	if strings.TrimSpace(config.Path) == "" {
		config.Path = "/swagger"
	}
}

func normalizeSlotRuntimeConfig(config *SlotRuntimeConfig) {
	if config == nil {
		return
	}
	if strings.TrimSpace(config.Image) == "" {
		config.Image = "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64"
	}
	if strings.TrimSpace(config.ContainerNamePrefix) == "" {
		config.ContainerNamePrefix = "private-browser-slot"
	}
	if config.InternalCDPPort <= 0 {
		config.InternalCDPPort = 9222
	}
	if config.InternalVNCPort <= 0 {
		config.InternalVNCPort = 5900
	}
	if config.HostCDPBasePort <= 0 {
		config.HostCDPBasePort = 9200
	}
	if config.HostVNCBasePort <= 0 {
		config.HostVNCBasePort = 9100
	}
}
