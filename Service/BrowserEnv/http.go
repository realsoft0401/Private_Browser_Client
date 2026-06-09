package BrowserEnv

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Pkg/HttpResponse"
	TaskService "private_browser_client/Service/Task"
)

// ListBrowserEnvs 查询本机环境包索引列表。
//
// HTTP 层只负责读取 query 参数并转给 Service；
// 查询逻辑、默认排除 deleted、分页上限和数据库访问都不在这里实现，避免接口层变成黑盒业务层。
func ListBrowserEnvs(c *gin.Context) {
	page, err := parseOptionalIntQuery(c, "page")
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "page 必须是整数")
		return
	}
	pageSize, err := parseOptionalIntQuery(c, "pageSize")
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "pageSize 必须是整数")
		return
	}

	result, err := NewService().ListBrowserEnvs(model.ListBrowserEnvQuery{
		UserID:   c.Query("userId"),
		RPAType:  c.Query("rpaType"),
		Status:   c.Query("status"),
		Page:     page,
		PageSize: pageSize,
	}, publicRequestBase(c), publicWebSocketBase(c))
	if err != nil {
		if bizErr, ok := IsBusinessError(err); ok {
			switch bizErr.Kind {
			case errorKindInvalid:
				HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, bizErr.Message)
			case errorKindNotFound:
				HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeNotFound, bizErr.Message)
			case errorKindConflict:
				HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeConflict, bizErr.Message)
			case errorKindRemote:
				HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, bizErr.Message)
			default:
				HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeServerBusy, bizErr.Message)
			}
			return
		}
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeServerBusy, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// RunBrowserEnv 启动本地浏览器环境包。
//
// HTTP 层只接收 envId 和少量生命周期参数；Docker 镜像、端口、代理、指纹和挂载都由 Service 从环境包读取。
// 这条接口不能演变成 Docker run 参数透传，否则会破坏环境包作为本机事实来源的设计。
// run 内部包含 Docker 创建、CDP/ curl timezone 探测和必要的二次重建，因此这里返回 SSE 任务，避免调用方等待到 socket hang up。
func RunBrowserEnv(c *gin.Context) {
	param, ok := bindOptionalRunRequest(c)
	if !ok {
		return
	}
	task := TaskService.Create("browser_env_run", "browser_env", c.Param("envId"), "浏览器环境启动任务已创建")
	go runBrowserEnvTask(task.ID, c.Param("envId"), param)
	HttpResponse.ResponseSuccess(c, TaskService.NewStartResponse(task, publicRequestBase(c)))
}

// StopBrowserEnv 停止本地浏览器环境包对应的 Docker 容器。
//
// HTTP 层只接收 envId 和 timeoutSeconds；容器 ID、容器名和状态回写都由 Service 从 SQLite 与环境包文件里确认。
// 这样前端不需要也不应该绕过环境包模型直接操作 Docker 容器。
// stop 可能等待浏览器写入 profile 和 Docker 停止超时，因此也走 SSE 任务通道。
func StopBrowserEnv(c *gin.Context) {
	param, ok := bindOptionalStopRequest(c)
	if !ok {
		return
	}
	task := TaskService.Create("browser_env_stop", "browser_env", c.Param("envId"), "浏览器环境停止任务已创建")
	go stopBrowserEnvTask(task.ID, c.Param("envId"), param)
	HttpResponse.ResponseSuccess(c, TaskService.NewStartResponse(task, publicRequestBase(c)))
}

// RevalidateBrowserEnv 对 error 环境执行受控重新准入。
//
// HTTP 层只创建 SSE 任务；真实校验在 Service 中完成，且不会启动容器或拉镜像。
// 这样管理员可以在排查后重新准入，但前端不能把它当成“直接恢复可用”的快捷按钮。
func RevalidateBrowserEnv(c *gin.Context) {
	task := TaskService.Create("browser_env_revalidate", "browser_env", c.Param("envId"), "浏览器环境重新校验任务已创建")
	go revalidateBrowserEnvTask(task.ID, c.Param("envId"))
	HttpResponse.ResponseSuccess(c, TaskService.NewStartResponse(task, publicRequestBase(c)))
}

// BackupBrowserEnv 执行“备份资产”动作。
//
// Service 会生成本机 tar.gz 备份包，删除容器和源环境目录，并把 SQLite 索引改成 backed_up。
// 这个项目仍处于开发期，不保留旧的临时打包下载接口，避免 backup 和 download 语义混乱。
func BackupBrowserEnv(c *gin.Context) {
	result, err := NewService().BackupBrowserEnv(c.Param("envId"))
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// RestoreBrowserEnv 从 SQLite 记录的本机备份包恢复环境目录。
//
// restore 只恢复文件并重置容器运行态，不启动 Docker；前端需要继续调用 run 执行 RPA。
func RestoreBrowserEnv(c *gin.Context) {
	result, err := NewService().RestoreBrowserEnv(c.Param("envId"))
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// ImportBrowserEnvPackage 从 tar.gz 包导入本机环境包。
//
// HTTP 层只负责接收 multipart 文件；包结构校验、checksum、路径安全、端口重分配和索引落库都在 Service 层完成。
func ImportBrowserEnvPackage(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "file 不能为空")
		return
	}
	defer file.Close()

	result, err := NewService().ImportBrowserEnvPackage(file, header)
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// ListBrowserEnvRebuildCandidates 只读扫描可重建 SQLite 索引的本机环境包目录。
//
// 该接口不写数据库、不修复文件，只返回候选状态和错误原因，避免坏目录自动进入系统。
func ListBrowserEnvRebuildCandidates(c *gin.Context) {
	result, err := NewService().ListBrowserEnvRebuildCandidates()
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// RebuildBrowserEnvIndex 从原子完整的环境包目录重建单个 SQLite 索引。
//
// 该接口不启动 Docker、不拉镜像、不创建容器；它只把 profile/binding/proxy/fingerprint/browser-data
// 校验通过的目录纳入 browser_envs 索引。
func RebuildBrowserEnvIndex(c *gin.Context) {
	result, err := NewService().RebuildBrowserEnvIndex(c.Param("envId"))
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// DeleteBrowserEnv 彻底删除本机浏览器环境包。
//
// HTTP 层只接收 envId；是否允许删除、running 冲突判断和索引更新都由 Service 完成。
// 这条接口会删除配置目录和 browser-data/profile，前端必须在调用前提示用户“删除后无法找回”。
// 删除涉及容器清理、物理目录删除和 SQLite 索引更新，改为 SSE 任务后前端能明确看到最终成功/失败。
func DeleteBrowserEnv(c *gin.Context) {
	task := TaskService.Create("browser_env_delete", "browser_env", c.Param("envId"), "浏览器环境删除任务已创建")
	go deleteBrowserEnvTask(task.ID, c.Param("envId"))
	HttpResponse.ResponseSuccess(c, TaskService.NewStartResponse(task, publicRequestBase(c)))
}

// GetBrowserEnvDetail 返回单个环境包详情。
//
// 详情接口读取 SQLite 索引和环境包文件，但不返回代理明文和指纹 raw；
// 后续代理重新配置会走独立修改接口，不能把详情接口扩展成“读取并编辑全部文件”的黑盒入口。
func GetBrowserEnvDetail(c *gin.Context) {
	result, err := NewService().GetBrowserEnvDetail(c.Param("envId"), publicRequestBase(c), publicWebSocketBase(c))
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetBrowserEnvVNCInfo 返回浏览器版 VNC 连接信息。
//
// 这个接口不返回 VNC 密码，因为当前容器内 x11vnc 是 -nopw；
// Mac 原生客户端弹密码的问题由浏览器 noVNC + WebSocket 代理绕开。
func GetBrowserEnvVNCInfo(c *gin.Context) {
	result, err := NewService().GetBrowserEnvVNCInfo(c.Param("envId"), publicRequestBase(c), publicWebSocketBase(c))
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// TestBrowserEnvCDP 返回环境包 CDP 端口的基础连通性诊断。
//
// 这个接口只做 CDP 自身测试，不访问 timezone provider、不判断代理出口；
// 它用于把“CDP 端口打不开”和“provider/规则链路失败”拆开排查。
func TestBrowserEnvCDP(c *gin.Context) {
	result, err := NewService().TestBrowserEnvCDP(c.Param("envId"))
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// UpdateBrowserEnvProxy 修改环境包代理配置。
//
// 这不是热更新接口：代理会进入容器启动环境变量，所以 running 环境会由后台任务重建容器。
// running 环境返回 taskId/eventsUrl，前端必须通过 SSE 观察后台重建和 timezone 探测结果。
func UpdateBrowserEnvProxy(c *gin.Context) {
	param, ok := bindStrictProxyUpdateRequest(c)
	if !ok {
		return
	}
	result, err := NewService().UpdateBrowserEnvProxy(c.Param("envId"), param)
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	attachTaskEventsURL(c, result)
	HttpResponse.ResponseSuccess(c, result)
}

func attachTaskEventsURL(c *gin.Context, result *model.UpdateBrowserEnvProxyResponse) {
	if result == nil || strings.TrimSpace(result.TaskID) == "" {
		return
	}
	result.EventsURL = publicRequestBase(c) + "/api/v1/edge/tasks/" + result.TaskID + "/events"
}

// UpdateBrowserEnvProxyMode 切换环境包 Clash 代理模式。
//
// 这条接口只修改 proxy/clash.yaml 的 mode 字段，不属于 run 参数；
// running 环境会由 Service 自动重建容器，并重新执行容器内 timezone 探测。
// 与 PATCH proxy 一样，running 环境会返回 taskId/eventsUrl，避免同步等待 rule/CDP 链路。
func UpdateBrowserEnvProxyMode(c *gin.Context) {
	param := new(model.UpdateBrowserEnvProxyModeRequest)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(param); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return
	}
	result, err := NewService().UpdateBrowserEnvProxyMode(c.Param("envId"), param)
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	attachTaskEventsURL(c, result)
	HttpResponse.ResponseSuccess(c, result)
}

// runBrowserEnvTask 在后台执行浏览器环境启动。
//
// 设计来源：
// - run 期间可能拉起容器、等待浏览器/CDP、探测 timezone，并在 timezone 变化后重建一次容器；
// - 这些步骤都可能超过 Apifox/前端 HTTP 超时时间，所以 HTTP handler 只创建任务，真实结果通过 SSE 返回。
func runBrowserEnvTask(taskID string, envID string, param *model.RunBrowserEnvRequest) {
	TaskService.MarkRunning(taskID, "browser_env_run", "开始启动浏览器环境", map[string]any{"envId": envID})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TaskService.RunHeartbeat(ctx, taskID, "browser_env_run", "浏览器环境启动仍在执行")
	result, err := NewService().RunBrowserEnv(envID, param)
	if err != nil {
		TaskService.Failed(taskID, "browser_env_run", err.Error())
		return
	}
	TaskService.Done(taskID, "browser_env_run", "浏览器环境启动完成", result)
}

// stopBrowserEnvTask 在后台执行浏览器环境停止。
//
// stop 需要给浏览器容器留出退出时间，不能因为 HTTP 客户端断开就中断状态回写。
func stopBrowserEnvTask(taskID string, envID string, param *model.StopBrowserEnvRequest) {
	TaskService.MarkRunning(taskID, "browser_env_stop", "开始停止浏览器环境", map[string]any{"envId": envID})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TaskService.RunHeartbeat(ctx, taskID, "browser_env_stop", "浏览器环境停止仍在执行")
	result, err := NewService().StopBrowserEnv(envID, param)
	if err != nil {
		TaskService.Failed(taskID, "browser_env_stop", err.Error())
		return
	}
	TaskService.Done(taskID, "browser_env_stop", "浏览器环境停止完成", result)
}

// revalidateBrowserEnvTask 在后台执行异常环境重新准入。
//
// revalidate 可能访问 Docker、扫描环境包文件并回写多个 JSON/SQLite 字段，任务化后前端能稳定看到最终失败原因。
func revalidateBrowserEnvTask(taskID string, envID string) {
	TaskService.MarkRunning(taskID, "browser_env_revalidate", "开始重新校验异常浏览器环境", map[string]any{"envId": envID})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TaskService.RunHeartbeat(ctx, taskID, "browser_env_revalidate", "浏览器环境重新校验仍在执行")
	result, err := NewService().RevalidateBrowserEnv(envID)
	if err != nil {
		TaskService.Failed(taskID, "browser_env_revalidate", err.Error())
		return
	}
	TaskService.Done(taskID, "browser_env_revalidate", "浏览器环境重新校验完成", result)
}

// deleteBrowserEnvTask 在后台执行彻底删除。
//
// 删除是不可恢复动作，前端负责二次确认；后端任务只保证删除过程可观察，并把失败原因留在任务事件里。
func deleteBrowserEnvTask(taskID string, envID string) {
	TaskService.MarkRunning(taskID, "browser_env_delete", "开始删除浏览器环境", map[string]any{"envId": envID})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TaskService.RunHeartbeat(ctx, taskID, "browser_env_delete", "浏览器环境删除仍在执行")
	result, err := NewService().DeleteBrowserEnv(envID)
	if err != nil {
		TaskService.Failed(taskID, "browser_env_delete", err.Error())
		return
	}
	TaskService.Done(taskID, "browser_env_delete", "浏览器环境删除完成", result)
}

// ProxyBrowserEnvVNC 把 noVNC WebSocket 流量代理到本机 VNC TCP 端口。
//
// noVNC 在浏览器中只能说 WebSocket，而容器里的 x11vnc 暴露的是 TCP；
// 这里承担 websockify 的角色，但目标端口必须从 browser_envs 索引读取，不能由前端传。
func ProxyBrowserEnvVNC(c *gin.Context) {
	if err := NewService().ProxyBrowserEnvVNC(c.Writer, c.Request, c.Param("envId")); err != nil {
		if !c.Writer.Written() {
			writeBrowserEnvError(c, err)
		}
	}
}

// CreateBrowserEnv 创建本地浏览器环境包。
//
// HTTP 层只负责 JSON 绑定、调用 Service 和统一响应。
// 这个接口当前不启动 Docker，不检查 CDP/VNC，只把服务端传来的配置落成本地环境包文件。
func CreateBrowserEnv(c *gin.Context) {
	param, ok := bindStrictCreateBrowserEnvRequest(c)
	if !ok {
		return
	}

	result, err := NewService().CreateBrowserEnv(param)
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// bindStrictCreateBrowserEnvRequest 绑定创建环境包请求，并拒绝未知字段。
//
// 设计来源：
// - 创建环境包是服务端下发完整 profile 的入口，字段含义必须稳定；
// - 当前项目是新开发，不兼容旧 proxy.config 明文入参；
// - 启用 DisallowUnknownFields 可以让旧字段立刻暴露为参数错误，避免“提交成功但配置没生效”的假象。
func bindStrictCreateBrowserEnvRequest(c *gin.Context) (*model.CreateBrowserEnvRequest, bool) {
	param := new(model.CreateBrowserEnvRequest)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(param); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return nil, false
	}
	return param, true
}

func writeBrowserEnvError(c *gin.Context, err error) {
	if bizErr, ok := IsBusinessError(err); ok {
		switch bizErr.Kind {
		case errorKindInvalid:
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, bizErr.Message)
		case errorKindNotFound:
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeNotFound, bizErr.Message)
		case errorKindConflict:
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeConflict, bizErr.Message)
		case errorKindRemote:
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, bizErr.Message)
		default:
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeServerBusy, bizErr.Message)
		}
		return
	}
	HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeServerBusy, err.Error())
}

// parseOptionalIntQuery 解析可选整数查询参数。
//
// 空字符串返回 0，由 Service 层按业务规则补默认值；
// 非数字在 HTTP 层提前返回参数错误，避免把明显的接口格式问题传到数据库查询阶段。
func parseOptionalIntQuery(c *gin.Context, key string) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}

// bindOptionalRunRequest 绑定 run 的可选 JSON 请求体。
//
// 空 body 表示按环境包当前配置直接启动；第一版只接受 forceRecreate，
// 不接受镜像、端口、挂载和 Docker HostConfig 透传。
func bindOptionalRunRequest(c *gin.Context) (*model.RunBrowserEnvRequest, bool) {
	param := new(model.RunBrowserEnvRequest)
	if err := c.ShouldBindJSON(param); err != nil {
		if errors.Is(err, io.EOF) {
			return param, true
		}
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return nil, false
	}
	return param, true
}

// bindOptionalStopRequest 绑定 stop 的可选 JSON 请求体。
//
// 空 body 表示使用 Service 的默认停止等待时间；这里不复用 Edge 层请求模型，
// 是为了让 BrowserEnv 生命周期接口拥有自己的协议边界，后续扩展也不会被底层 Docker API 牵着走。
func bindOptionalStopRequest(c *gin.Context) (*model.StopBrowserEnvRequest, bool) {
	param := new(model.StopBrowserEnvRequest)
	if err := c.ShouldBindJSON(param); err != nil {
		if errors.Is(err, io.EOF) {
			return param, true
		}
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return nil, false
	}
	return param, true
}

// bindStrictProxyUpdateRequest 绑定代理修改请求，并拒绝未知字段。
//
// 设计来源：
// - 当前项目是新开发，不需要兼容早期明文 config 字段；
// - Go 默认 JSON 解码会忽略未知字段，这会让旧 config 看似提交成功但实际不生效；
// - 这里对 PATCH proxy 单独启用 DisallowUnknownFields，确保正式协议只接受 configBase64。
func bindStrictProxyUpdateRequest(c *gin.Context) (*model.UpdateBrowserEnvProxyRequest, bool) {
	param := new(model.UpdateBrowserEnvProxyRequest)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(param); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return nil, false
	}
	return param, true
}

func publicRequestBase(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := c.Request.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return scheme + "://" + host
}

func publicWebSocketBase(c *gin.Context) string {
	httpBase := publicRequestBase(c)
	if strings.HasPrefix(httpBase, "https://") {
		return "wss://" + strings.TrimPrefix(httpBase, "https://")
	}
	return "ws://" + strings.TrimPrefix(httpBase, "http://")
}

// WebVNCPage 返回独立浏览器 VNC 页面。
//
// 这不是恢复旧静态控制台，只是为 Mac/浏览器访问 VNC 提供一个最小页面。
func WebVNCPage(c *gin.Context) {
	c.File("public/web-vnc.html")
}
