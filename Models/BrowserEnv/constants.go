package BrowserEnv

// 环境包模型层常量单独拆文件，原因是 browser_env.go 已经超过 900 行。
// 这类常量会被请求模型、索引模型和运行流程同时复用，继续和所有结构体堆在一起会让维护入口越来越模糊。

const (
	// SchemaVersion 是环境包文件格式版本。
	//
	// 设计来源：
	// - 环境包后续要支持云存储、跨设备导入和长期迁移；
	// - schemaVersion 写进 profile 后，未来字段升级时可以做兼容判断；
	// - 第一版固定为 1，不允许调用方传入覆盖。
	SchemaVersion = 1

	// DefaultContainerUserDataDir 是容器内 Chromium 用户数据目录。
	//
	// 登录态真实载体会挂载到这个路径；后续不能随便改成临时目录，
	// 否则 Cookies、LocalStorage、IndexedDB 等登录态数据无法复用。
	DefaultContainerUserDataDir = "/data/profile"
	DefaultStartupURL           = "about:blank"
	DefaultShmSize              = "1g"
	DefaultScreenDepth          = 24
)

const (
	// BrowserEnvStatusCreated 表示环境包已建立，但还没有真正启动容器。
	BrowserEnvStatusCreated = "created"
	// BrowserEnvStatusRunning 表示环境包已进入运行态。
	BrowserEnvStatusRunning = "running"
	// BrowserEnvStatusStopped 表示最近一次容器已经停止。
	BrowserEnvStatusStopped = "stopped"
	// BrowserEnvStatusBackedUp 表示环境包已经备份为 tar.gz，运行目录和容器已释放。
	//
	// 设计来源：
	// - 用户确认 RPA 执行后只保留备份文件，下一次执行前再恢复环境包；
	// - 因此 SQLite 记录不能删除，而要从“可运行目录索引”变成“环境资产索引”；
	// - 处于该状态时不能直接 run，必须先 restore 把 browser-envs 目录恢复出来。
	BrowserEnvStatusBackedUp = "backed_up"
	// BrowserEnvStatusDeleted 表示逻辑删除，不应再作为正常列表展示。
	BrowserEnvStatusDeleted = "deleted"
	// BrowserEnvStatusArchived 表示已归档，可保留文件但不参与活跃列表。
	BrowserEnvStatusArchived = "archived"
	// BrowserEnvStatusError 表示创建或运行过程发生异常。
	BrowserEnvStatusError = "error"

	// BrowserEnvContainerStatusUnknown 表示数据库里还没有容器事实快照。
	BrowserEnvContainerStatusUnknown = "unknown"
	// BrowserEnvMonitorStatusUnknown 表示监控尚未上报。
	BrowserEnvMonitorStatusUnknown = "unknown"
)

// SupportedRPATypes 是第一版允许的 RPA 类型。
//
// 这里保留 tk/fb/ins 等短码，是为了让 envId、云存储 key 和目录结构保持短而稳定。
// 如果未来新增平台，应先在这里扩展枚举，再同步更新 OpenAPI 和项目文档。
var SupportedRPATypes = map[string]struct{}{
	"tk":    {},
	"fb":    {},
	"ins":   {},
	"yt":    {},
	"x":     {},
	"other": {},
}
