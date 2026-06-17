package BrowserEnv

// 这组常量描述 browser-env 正式资产层的固定协议。
//
// 设计来源：
// - 当前项目已经把正式生命周期收口到 `browser-envs/*`；
// - 文档已经明确 profile/binding/container 是本机环境包真实文件协议；
// - 因此这些常量不能再散落到 run/stop/create 各个流程里各写一份。
//
// 职责边界：
// - 这里只定义正式资产层的稳定枚举和值；
// - 不负责 HTTP 参数校验，不负责 Docker 运行逻辑；
// - 后续如果状态枚举调整，必须先同步文档，再改这里和写库逻辑。
const (
	SchemaVersion               = 1
	FixedLanguage               = "us-en"
	DefaultContainerUserDataDir = "/data/profile"
	DefaultStartupURL           = "about:blank"
	DefaultShmSize              = "1g"
	DefaultScreenDepth          = 24
)

const (
	BrowserEnvStatusCreated  = "created"
	BrowserEnvStatusRunning  = "running"
	BrowserEnvStatusStopped  = "stopped"
	BrowserEnvStatusBackedUp = "backed_up"
	BrowserEnvStatusDeleted  = "deleted"
	BrowserEnvStatusError    = "error"
)

const (
	ContainerStatusCreated = "created"
	ContainerStatusRunning = "running"
	ContainerStatusExited  = "exited"
	ContainerStatusMissing = "missing"
	ContainerStatusError   = "error"
)

const (
	MonitorStatusUnknown = "unknown"
)

// SupportedRPATypes 是当前 Client 接受的环境平台短码。
var SupportedRPATypes = map[string]struct{}{
	"tk":    {},
	"fb":    {},
	"ins":   {},
	"yt":    {},
	"x":     {},
	"other": {},
}
