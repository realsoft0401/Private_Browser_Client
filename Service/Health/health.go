package Health

import (
	"time"

	sqliteInfra "private_browser_client/Infrastructures/SQLite"
	"private_browser_client/Settings"
)

const (
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
)

type CheckResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type Response struct {
	OK         bool                   `json:"ok"`
	Status     string                 `json:"status"`
	Service    string                 `json:"service"`
	Mode       string                 `json:"mode"`
	Version    string                 `json:"version"`
	ConfigFile string                 `json:"configFile"`
	DockerAPI  string                 `json:"dockerApi"`
	CheckedAt  int64                  `json:"checkedAt"`
	Checks     map[string]CheckResult `json:"checks"`
}

// BuildHealthResponse 汇总当前新 Client 的最小健康检查结果。
//
// 当前阶段先按 old 的边界保留本机视角 `healthy/unhealthy`，
// 不引入 Server 侧的 `offline/stale` 语义。
//
// 注意：
//   - Node 登记协同能力属于“是否具备与中心查询/接收 assign 的配置条件”；
//   - 它不是本机中心身份事实，也不能要求 /health 因为没拿到 clientId 就判 unhealthy，
//     否则又会把 Client 和中心身份强绑定回去。
func BuildHealthResponse() Response {
	sqliteStatus := StatusUnhealthy
	sqliteMessage := "sqlite 未初始化"
	if sqliteInfra.DB() != nil {
		sqliteStatus = StatusHealthy
		sqliteMessage = "sqlite 已初始化"
	}
	nodeRegisterEnabled := Settings.Conf.NodeRegisterConfig != nil && Settings.Conf.NodeRegisterConfig.Enabled
	registrationStatus := StatusUnhealthy
	registrationMessage := "node_register 未启用；当前 Client 仍可作为独立边缘服务运行"
	if nodeRegisterEnabled {
		registrationMessage = "node_register 配置不完整"
	}
	if nodeRegisterConfigReady() {
		registrationStatus = StatusHealthy
		registrationMessage = "Node 登记协同配置已就绪，可查询中心状态并接收 assign"
	}
	checks := map[string]CheckResult{
		"api":              {Status: StatusHealthy, Message: "http 服务可响应"},
		"sqlite":           {Status: sqliteStatus, Message: sqliteMessage},
		"swagger":          {Status: StatusHealthy, Message: "swagger/openapi 入口已挂载"},
		"nodeRegistration": {Status: registrationStatus, Message: registrationMessage},
	}

	return Response{
		OK:         true,
		Status:     StatusHealthy,
		Service:    Settings.Conf.Name,
		Mode:       Settings.Conf.Mode,
		Version:    Settings.Conf.Version,
		ConfigFile: Settings.Conf.ConfigFile,
		DockerAPI:  Settings.Conf.DockerConfig.APIURL,
		CheckedAt:  time.Now().Unix(),
		Checks:     checks,
	}
}

func nodeRegisterConfigReady() bool {
	config := Settings.Conf.NodeRegisterConfig
	if config == nil || !config.Enabled {
		return false
	}
	return config.ServerBaseURL != "" && config.MainAccountID != ""
}
