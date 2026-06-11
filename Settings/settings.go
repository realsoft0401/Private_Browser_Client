package Settings

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

var (
	// BuildEnv 是旧版多环境构建参数的兼容占位。
	//
	// 当前 Client 已收紧为只读取 `Settings/config-docker.yaml`，不再根据 BuildEnv/ENV 选择配置文件。
	// 变量保留是为了不破坏旧 Dockerfile 里的 ldflags；后续清理构建脚本时可以一起移除。
	BuildEnv = ""

	// ConfigFileName 是 Edge Client 唯一允许读取的配置文件名。
	//
	// 设计来源：用户确认 Settings 目录只保留 config-docker.yaml，Mac 本地、Linux 节点、Docker 容器
	// 都使用同一套生产口径配置；差异通过挂载覆盖这个文件表达，不再引入 dev/test/prod 三套配置。
	ConfigFileName = "config-docker.yaml"

	// Conf 持有当前服务进程的全局配置。
	Conf = new(AppConfig)

	configEngine = viper.New()
)

type AppConfig struct {
	Name    string `mapstructure:"name"`
	Mode    string `mapstructure:"mode"`
	Version string `mapstructure:"version"`
	// ProjectRoot / ConfigFile / Env 是运行期补进去的元信息，不从 yaml 直接反序列化。
	// 它们主要服务于排障和健康检查，避免服务起来后还不知道自己到底读了哪份配置。
	ProjectRoot       string `mapstructure:"-"`
	ConfigFile        string `mapstructure:"-"`
	Env               string `mapstructure:"-"`
	*ServerConfig     `mapstructure:"server"`
	*DockerConfig     `mapstructure:"docker"`
	*StatusSyncConfig `mapstructure:"status_sync"`
	*DiscoveryConfig  `mapstructure:"discovery"`
}

// ServerConfig 描述当前 Go 边缘服务自身的监听参数。
// 它不表示中心服务端地址，后续不要把上报目标、中心节点配置和本机监听配置混写到一起。
type ServerConfig struct {
	Host                string `mapstructure:"host"`
	Port                int    `mapstructure:"port"`
	ReadTimeoutSeconds  int    `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `mapstructure:"write_timeout_seconds"`
}

// DockerConfig 描述边缘服务访问本机 Docker Engine 的方式。
//
// 设计来源：
// - `Private_Browser_Client` 已重新定位为边缘服务，不再保存多节点列表；
// - 边缘服务只读取和管理本机 Docker，所以这里配置的是本机 Docker API 地址；
// - 默认使用 Docker Engine HTTP 2375，后续如果改成本地 socket 或 TLS 2376，应只扩展这一配置和 Edge Service。
type DockerConfig struct {
	APIURL string `mapstructure:"api_url"`
}

// StatusSyncConfig 描述浏览器环境运行态后台同步任务。
//
// 设计来源：
// - 用户明确要求边缘服务每隔几秒获取一次容器状态并刷新数据库；
// - 用户进一步要求定时任务必须有哨兵机制，任务挂掉后要自动拉起来；
// - 这些值放进配置层，是为了后续在开发、部署和排障时能调整频率，而不是把时间常量散落在任务代码里。
//
// 职责边界：
// - 这里只描述任务节奏，不描述具体同步业务；
// - Worker 只同步状态，不启动/停止/删除浏览器容器；
// - Watchdog 只守护同步任务本身，不守护 Docker 容器。
type StatusSyncConfig struct {
	Enabled         bool `mapstructure:"enabled"`
	IntervalSeconds int  `mapstructure:"interval_seconds"`
	WatchdogSeconds int  `mapstructure:"watchdog_seconds"`
	StaleSeconds    int  `mapstructure:"stale_seconds"`
}

// DiscoveryConfig 描述 Client 在独立内网中的 UDP 服务发现广播。
//
// 设计来源：
// - 用户希望 Server 可以自动扫描内网并加入 Client，也可以手动添加；
// - 用户进一步确认 UDP 广播必须有唯一识别口径，不能把内网里所有 UDP 包都抓来当节点；
// - 当前收窄后不再使用 clientInstanceId，Client IP 是连接位置，clientId 由 Node Server 发放。
//
// 职责边界：
// - 只广播本 Client 的服务位置、协议版本、分组和基础能力；
// - 不广播用户、环境包、代理、指纹、登录态或宿主机敏感环境变量；
// - IP 变化时由 Server 发现不一致后提示人工更新，不由 Client 自动覆盖中心 clientId 身份。
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

// Init 负责加载当前部署配置文件。
//
// 这个配置入口的来历：
// - 早期为了本地开发区分过 dev/test/prod 配置文件；
// - 用户在 2026-06-10 明确收紧：Settings 目录只保留 config-docker.yaml，go run 和 Docker 都必须读它；
// - 因此这里不再读取 ENV/BuildEnv，也不再拼 config-prod.yaml/config-dev.yaml/config-test.yaml。
func Init(projectRoot string) error {
	configFile := filepath.Join(projectRoot, "Settings", ConfigFileName)

	configEngine = viper.New()
	configEngine.SetConfigFile(configFile)
	configEngine.SetConfigType("yaml")
	configEngine.SetDefault("name", "private-browser-client")
	configEngine.SetDefault("mode", "production")
	configEngine.SetDefault("version", "0.1.9")
	configEngine.SetDefault("server.host", "0.0.0.0")
	configEngine.SetDefault("server.port", 3300)
	configEngine.SetDefault("server.read_timeout_seconds", 15)
	configEngine.SetDefault("server.write_timeout_seconds", 15)
	configEngine.SetDefault("docker.api_url", "http://127.0.0.1:2375")
	configEngine.SetDefault("status_sync.enabled", true)
	configEngine.SetDefault("status_sync.interval_seconds", 5)
	configEngine.SetDefault("status_sync.watchdog_seconds", 15)
	configEngine.SetDefault("status_sync.stale_seconds", 30)
	configEngine.SetDefault("discovery.enabled", true)
	configEngine.SetDefault("discovery.broadcast_address", "255.255.255.255")
	configEngine.SetDefault("discovery.port", 43000)
	configEngine.SetDefault("discovery.interval_seconds", 5)
	configEngine.SetDefault("discovery.magic", "PRIVATE_BROWSER_CLIENT_DISCOVERY")
	configEngine.SetDefault("discovery.protocol_version", 1)
	configEngine.SetDefault("discovery.group", "default")

	if err := configEngine.ReadInConfig(); err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}
	if err := configEngine.Unmarshal(Conf); err != nil {
		return fmt.Errorf("unmarshal config failed: %w", err)
	}

	Conf.ProjectRoot = projectRoot
	Conf.ConfigFile = configFile
	Conf.Env = "docker"
	normalizeRuntimeMode(Conf)
	if Conf.ServerConfig == nil {
		Conf.ServerConfig = &ServerConfig{}
	}
	if Conf.DockerConfig == nil {
		Conf.DockerConfig = &DockerConfig{}
	}
	if Conf.StatusSyncConfig == nil {
		Conf.StatusSyncConfig = &StatusSyncConfig{}
	}
	if Conf.DiscoveryConfig == nil {
		Conf.DiscoveryConfig = &DiscoveryConfig{}
	}
	normalizeStatusSyncConfig(Conf.StatusSyncConfig)
	normalizeDiscoveryConfig(Conf.DiscoveryConfig)

	configEngine.WatchConfig()
	// 这里保留热加载，是因为配置文件在本地开发和部署阶段都可能被手动修改。
	// 但要注意：当前只更新内存配置对象，不自动重启 HTTP Server 或重新连库，避免运行中出现不可控副作用。
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
		if updated.StatusSyncConfig == nil {
			updated.StatusSyncConfig = &StatusSyncConfig{}
		}
		if updated.DiscoveryConfig == nil {
			updated.DiscoveryConfig = &DiscoveryConfig{}
		}
		normalizeStatusSyncConfig(updated.StatusSyncConfig)
		normalizeDiscoveryConfig(updated.DiscoveryConfig)
		Conf = updated
		fmt.Printf("config reloaded: %s\n", event.Name)
	})

	return nil
}

// normalizeRuntimeMode 把边缘服务运行口径统一为 production。
//
// 设计来源：
// - Client 是部署在客户节点上的边缘服务，不应因为读取 config-dev.yaml/config-test.yaml 就进入不同业务模式；
// - 配置文件名只表示“这次启动读取哪份参数”，不应影响 SQLite、生命周期状态机、导入恢复规则或接口语义；
// - 这样可以避免后续切换 ENV 后出现不同 mode、不同数据库、不同状态口径导致的“数据掉了”错觉。
func normalizeRuntimeMode(config *AppConfig) {
	if config == nil {
		return
	}
	config.Mode = "production"
}

// normalizeStatusSyncConfig 收敛后台同步任务的时间配置。
//
// 这里给出保守下限，是为了避免配置误写成 0 或 1 秒后疯狂请求 Docker API；
// 同时保证 staleSeconds 大于 watchdogSeconds，否则哨兵会过于频繁地误判 Worker 卡死。
func normalizeStatusSyncConfig(config *StatusSyncConfig) {
	if config == nil {
		return
	}
	if config.IntervalSeconds <= 0 {
		config.IntervalSeconds = 5
	}
	if config.IntervalSeconds < 3 {
		config.IntervalSeconds = 3
	}
	if config.WatchdogSeconds <= 0 {
		config.WatchdogSeconds = 15
	}
	if config.WatchdogSeconds < 5 {
		config.WatchdogSeconds = 5
	}
	if config.StaleSeconds <= 0 {
		config.StaleSeconds = 30
	}
	minStale := config.WatchdogSeconds * 2
	if config.StaleSeconds < minStale {
		config.StaleSeconds = minStale
	}
}

// normalizeDiscoveryConfig 收敛 UDP discovery 配置。
//
// 这里给出明确默认值，是为了让 Server 自动发现时有稳定协议识别符；
// magic/group/protocolVersion 是过滤依据，不承担认证职责，Client 仍依赖内网隔离和上游访问控制。
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
		config.IntervalSeconds = 5
	}
	if config.IntervalSeconds < 3 {
		config.IntervalSeconds = 3
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
