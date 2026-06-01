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
	// BuildEnv 用于在编译阶段注入默认环境，例如：
	// go build -ldflags "-X private_browser_client/Settings.BuildEnv=prod"
	BuildEnv = ""

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

// Init 负责加载当前运行环境的配置文件。
//
// 这个配置入口的来历：
// - 用户之前已经明确提出需要 dev / test / prod 三套环境配置；
// - 现在项目从桌面端切回纯服务端后，这个入口继续承担“按环境装配配置”的职责；
// - 后续新增 Redis、任务队列或第三方服务时，也应继续通过这里统一挂载，避免业务层直接读环境变量形成隐式依赖。
func Init(projectRoot string) error {
	env := resolveEnv()
	configFile := filepath.Join(projectRoot, "Settings", fmt.Sprintf("config-%s.yaml", env))

	configEngine = viper.New()
	configEngine.SetConfigFile(configFile)
	configEngine.SetConfigType("yaml")
	configEngine.SetDefault("name", "private-browser-client")
	configEngine.SetDefault("mode", env)
	configEngine.SetDefault("version", "0.1.5")
	configEngine.SetDefault("server.host", "0.0.0.0")
	configEngine.SetDefault("server.port", 3300)
	configEngine.SetDefault("server.read_timeout_seconds", 15)
	configEngine.SetDefault("server.write_timeout_seconds", 15)
	configEngine.SetDefault("docker.api_url", "http://127.0.0.1:2375")
	configEngine.SetDefault("status_sync.enabled", true)
	configEngine.SetDefault("status_sync.interval_seconds", 5)
	configEngine.SetDefault("status_sync.watchdog_seconds", 15)
	configEngine.SetDefault("status_sync.stale_seconds", 30)

	if err := configEngine.ReadInConfig(); err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}
	if err := configEngine.Unmarshal(Conf); err != nil {
		return fmt.Errorf("unmarshal config failed: %w", err)
	}

	Conf.ProjectRoot = projectRoot
	Conf.ConfigFile = configFile
	Conf.Env = env
	if Conf.ServerConfig == nil {
		Conf.ServerConfig = &ServerConfig{}
	}
	if Conf.DockerConfig == nil {
		Conf.DockerConfig = &DockerConfig{}
	}
	if Conf.StatusSyncConfig == nil {
		Conf.StatusSyncConfig = &StatusSyncConfig{}
	}
	normalizeStatusSyncConfig(Conf.StatusSyncConfig)

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
		updated.Env = env
		if updated.ServerConfig == nil {
			updated.ServerConfig = &ServerConfig{}
		}
		if updated.DockerConfig == nil {
			updated.DockerConfig = &DockerConfig{}
		}
		if updated.StatusSyncConfig == nil {
			updated.StatusSyncConfig = &StatusSyncConfig{}
		}
		normalizeStatusSyncConfig(updated.StatusSyncConfig)
		Conf = updated
		fmt.Printf("config reloaded: %s\n", event.Name)
	})

	return nil
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

// resolveEnv 统一决定当前运行环境。
//
// 优先级保持成：运行时 `ENV` > 编译时 `BuildEnv` > 默认 `dev`。
// 这个顺序是为了兼顾本地开发、CI 构建和部署覆盖，后续不要在别处再写一套环境判断逻辑。
func resolveEnv() string {
	env := strings.TrimSpace(os.Getenv("ENV"))
	if env == "" {
		env = strings.TrimSpace(BuildEnv)
	}
	if env == "" {
		env = "dev"
	}
	return strings.ToLower(env)
}
