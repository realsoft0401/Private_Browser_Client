package Health

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"private_browser_client/Infrastructures/SQLite"
	browserEnvModel "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	BrowserEnvService "private_browser_client/Service/BrowserEnv"
	EdgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

const (
	// StatusHealthy 表示 Client 本机依赖都可用，Server 可以把该节点视为可操作候选。
	StatusHealthy = "healthy"
	// StatusUnhealthy 表示 Client 进程仍在线，但 Docker、SQLite、数据目录或后台同步存在阻塞性问题。
	StatusUnhealthy = "unhealthy"
)

// CheckResult 是 /health 中每个依赖项的检查结果。
//
// 设计来源：
// - 用户确认 Server 侧需要识别 healthy / unhealthy / offline / stale；
// - Client 进程本身只能判断“本机是否带病”，不能判断 Server 缓存是否 stale，也不能替 Server 判断 offline；
// - 因此这里把每个本机依赖拆成可读检查项，方便部署时看到错误就知道先修 Docker、SQLite 还是目录权限。
type CheckResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Response 是 Client /health 的稳定响应结构。
//
// 职责边界：
// - 只暴露本机运行健康、配置摘要、Docker 设备摘要和状态同步任务快照；
// - 不返回用户、环境包、代理、指纹和登录态数据；
// - 不生成 offline/stale，Server 应根据心跳超时或缓存不一致自行标记。
type Response struct {
	OK         bool                               `json:"ok"`
	Status     string                             `json:"status"`
	Service    string                             `json:"service"`
	Mode       string                             `json:"mode"`
	Version    string                             `json:"version"`
	ConfigFile string                             `json:"configFile"`
	DockerAPI  string                             `json:"dockerApi"`
	CheckedAt  int64                              `json:"checkedAt"`
	DeviceInfo *edgeModel.DeviceInfo              `json:"deviceInfo,omitempty"`
	Checks     map[string]CheckResult             `json:"checks"`
	StatusSync browserEnvModel.StatusSyncSnapshot `json:"statusSync"`
}

// BuildHealthResponse 汇总 Client 本机健康状态。
//
// 这里的判断体现当前收窄后的边缘服务口径：
// - HTTP 进程能响应只代表在线，不代表可用；
// - Docker 不可达、SQLite 不可写、数据目录异常、状态同步 Worker 挂掉，都应返回 unhealthy；
// - 但 offline / stale 是 Server 站在中心缓存角度做的判断，不能由 Client 自己伪造。
func BuildHealthResponse() Response {
	checks := map[string]CheckResult{
		"api": {Status: StatusHealthy, Message: "http 服务可响应"},
	}
	checkedAt := time.Now().Unix()

	checkDataDir(checks)
	checkSQLite(checks)
	deviceInfo := checkDocker(checks)
	statusSync := BrowserEnvService.StatusSyncSnapshot()
	checkStatusSync(checks, statusSync, checkedAt)

	status := StatusHealthy
	for _, check := range checks {
		if check.Status != StatusHealthy {
			status = StatusUnhealthy
			break
		}
	}

	return Response{
		OK:         status == StatusHealthy,
		Status:     status,
		Service:    Settings.Conf.Name,
		Mode:       Settings.Conf.Mode,
		Version:    Settings.Conf.Version,
		ConfigFile: Settings.Conf.ConfigFile,
		DockerAPI:  Settings.Conf.DockerConfig.APIURL,
		CheckedAt:  checkedAt,
		DeviceInfo: deviceInfo,
		Checks:     checks,
		StatusSync: statusSync,
	}
}

// checkDataDir 确认本机 data 目录存在且可进入。
//
// 环境包、SQLite 和备份索引都依赖该目录；如果目录缺失或权限异常，继续 run/backup/delete 会制造更难排查的半状态。
func checkDataDir(checks map[string]CheckResult) {
	dataDir := filepath.Join(Settings.Conf.ProjectRoot, "data")
	stat, err := os.Stat(dataDir)
	if err != nil {
		checks["dataDir"] = CheckResult{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("data 目录不可访问: %v；请确认项目目录权限，并确保 %s 存在", err, dataDir),
		}
		return
	}
	if !stat.IsDir() {
		checks["dataDir"] = CheckResult{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("data 路径不是目录: %s；请移除异常文件并重新创建目录", dataDir),
		}
		return
	}
	checks["dataDir"] = CheckResult{Status: StatusHealthy, Message: "data 目录可访问"}
}

// checkSQLite 只检查本机索引库连接是否可用。
//
// SQLite 是环境包索引事实源，但不保存 profile、代理明文或登录态；这里不能读取环境包业务数据，只做连接级健康判断。
func checkSQLite(checks map[string]CheckResult) {
	if SQLite.DB == nil {
		checks["sqlite"] = CheckResult{
			Status:  StatusUnhealthy,
			Message: "SQLite 未初始化；请检查启动日志中的 init sqlite/migrate 错误",
		}
		return
	}
	if err := SQLite.DB.Ping(); err != nil {
		checks["sqlite"] = CheckResult{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("SQLite ping 失败: %v；请检查 data 目录权限和数据库文件是否被占用", err),
		}
		return
	}
	checks["sqlite"] = CheckResult{Status: StatusHealthy, Message: "SQLite 可用"}
}

// checkDocker 通过 Edge Service 检查本机 Docker 2375。
//
// Docker 是 run/stop/backup/restore 等生命周期动作的硬依赖；不可达时必须明确 unhealthy，
// 不能静默降级为文件扫描，也不能切换到 SSH 或其它隐式管理方式。
func checkDocker(checks map[string]CheckResult) *edgeModel.DeviceInfo {
	deviceInfo, err := EdgeService.NewEdgeService().GetDeviceInfo()
	if err != nil {
		checks["docker"] = CheckResult{
			Status: StatusUnhealthy,
			Message: fmt.Sprintf(
				"Docker API 不可用: %v；请确认 Docker Engine 已开启 HTTP 2375，且 docker.api_url=%s 只能暴露在本机或独立内网",
				err,
				Settings.Conf.DockerConfig.APIURL,
			),
		}
		return nil
	}
	checks["docker"] = CheckResult{
		Status:  StatusHealthy,
		Message: fmt.Sprintf("Docker 可用，arch=%s，containers=%d，images=%d", deviceInfo.DeviceArch, deviceInfo.LastContainersCount, deviceInfo.LastImagesCount),
	}
	return deviceInfo
}

// checkStatusSync 判断后台状态同步任务是否仍在工作。
//
// 该任务只修正运行态摘要，不能改 profile、不能重建登录态、不能替代生命周期接口；
// 但如果 Worker 已停止或心跳过旧，Server 看到该节点时应把它当成 unhealthy，不允许带病操作。
func checkStatusSync(checks map[string]CheckResult, snapshot browserEnvModel.StatusSyncSnapshot, now int64) {
	if !snapshot.Enabled {
		checks["statusSync"] = CheckResult{Status: StatusHealthy, Message: "状态同步已按配置关闭"}
		return
	}
	if !snapshot.WorkerRunning {
		checks["statusSync"] = CheckResult{
			Status:  StatusUnhealthy,
			Message: "状态同步 Worker 未运行；请检查服务日志和 status_sync 配置",
		}
		return
	}
	if snapshot.LastHeartbeatAt != nil && snapshot.StaleSeconds > 0 && now-*snapshot.LastHeartbeatAt > int64(snapshot.StaleSeconds) {
		checks["statusSync"] = CheckResult{
			Status:  StatusUnhealthy,
			Message: "状态同步 Worker 心跳已过期；请检查 Docker API、SQLite 写入和 Worker 日志",
		}
		return
	}
	checks["statusSync"] = CheckResult{Status: StatusHealthy, Message: "状态同步 Worker 正常"}
}
