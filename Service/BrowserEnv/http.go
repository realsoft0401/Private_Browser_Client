package BrowserEnv

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Pkg/HttpResponse"
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
func RunBrowserEnv(c *gin.Context) {
	param, ok := bindOptionalRunRequest(c)
	if !ok {
		return
	}
	result, err := NewService().RunBrowserEnv(c.Param("envId"), param)
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

// StopBrowserEnv 停止本地浏览器环境包对应的 Docker 容器。
//
// HTTP 层只接收 envId 和 timeoutSeconds；容器 ID、容器名和状态回写都由 Service 从 SQLite 与环境包文件里确认。
// 这样前端不需要也不应该绕过环境包模型直接操作 Docker 容器。
func StopBrowserEnv(c *gin.Context) {
	param, ok := bindOptionalStopRequest(c)
	if !ok {
		return
	}
	result, err := NewService().StopBrowserEnv(c.Param("envId"), param)
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// BackupBrowserEnvPackage 生成环境包备份 tar.gz。
//
// 备份接口只返回下载流，不删除源环境包、Docker 容器或 SQLite 索引；
// Service 会拒绝 running 状态，避免 browser-data/profile 在写入过程中被打包。
func BackupBrowserEnvPackage(c *gin.Context) {
	result, err := NewService().BackupBrowserEnvPackage(c.Param("envId"))
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	if result.Cleanup != nil {
		defer result.Cleanup()
	}
	c.Header("Content-Type", "application/gzip")
	c.FileAttachment(result.FilePath, result.FileName)
}

// ExportAndRemoveBrowserEnvPackage 导出环境包并从本机移除源环境。
//
// 这条接口用于迁移场景：下载包生成成功后，Service 会删除已停止容器、环境包目录和 SQLite 索引；
// 前端必须在调用前提示用户“导出后本机不再保留该环境包”。
func ExportAndRemoveBrowserEnvPackage(c *gin.Context) {
	result, err := NewService().ExportAndRemoveBrowserEnvPackage(c.Param("envId"))
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	if result.Cleanup != nil {
		defer result.Cleanup()
	}
	c.Header("Content-Type", "application/gzip")
	c.FileAttachment(result.FilePath, result.FileName)
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

// DeleteBrowserEnv 彻底删除本机浏览器环境包。
//
// HTTP 层只接收 envId；是否允许删除、running 冲突判断和索引更新都由 Service 完成。
// 这条接口会删除配置目录和 browser-data/profile，前端必须在调用前提示用户“删除后无法找回”。
func DeleteBrowserEnv(c *gin.Context) {
	result, err := NewService().DeleteBrowserEnv(c.Param("envId"))
	if err != nil {
		writeBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
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

// UpdateBrowserEnvProxy 修改环境包代理配置。
//
// 这不是热更新接口：代理会进入容器启动环境变量，所以运行中环境包会被 Service 拒绝修改。
// 调用方应按 stop -> update proxy -> run 的顺序让配置真正生效。
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
	HttpResponse.ResponseSuccess(c, result)
}

// UpdateBrowserEnvProxyMode 切换环境包 Clash 代理模式。
//
// 这条接口只修改 proxy/clash.yaml 的 mode 字段，不属于 run 参数；
// running 环境会由 Service 自动重建容器，并重新执行容器内 timezone 探测。
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
	HttpResponse.ResponseSuccess(c, result)
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
