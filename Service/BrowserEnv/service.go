package BrowserEnv

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Settings"
)

var createEnvMu = sync.Mutex{}

type Service struct{}

// NewService 创建浏览器环境包服务。
//
// 当前 Service 负责本机环境包文件落盘、SQLite 轻量索引，以及按 envId 恢复 Docker 浏览器容器。
// Docker 镜像选择仍由中心服务端决定；边缘服务只执行环境包内的 runtime.image。
func NewService() *Service {
	return &Service{}
}

// createContext 是创建环境包过程中的内部上下文。
//
// 它把本次创建中生成的 envId、bindingId、envSequence、端口、hash 和路径集中保存，
// 避免主流程在多个局部变量之间来回传递，后续排查时也更容易确认每一步用的是同一套数据。
type createContext struct {
	Param           *model.CreateBrowserEnvRequest
	Now             int64
	SnowflakeID     string
	EnvID           string
	BindingID       string
	EnvSequence     int
	Ports           model.BrowserEnvPorts
	Paths           model.ManifestPaths
	RelativeEnvPath string
	AbsoluteEnvPath string
	ProxyConfigHash string
	Identity        model.BindingIdentity
	IdentityHash    string
	ConfigHash      string
}

// CreateBrowserEnv 创建一个本地浏览器环境包。
//
// 设计来源：
// - 用户明确修正开发顺序：当前不是先 run 容器，而是先把服务端数据保存成本地环境包；
// - 环境包要能整体打包迁移，因此 profile、binding、fingerprint、proxy、browser-data 必须放在同一目录；
// - envSequence 和端口由边缘服务本机 +1 生成，但不参与 identityHash，迁移导入时可以重分配。
//
// 职责边界：
// - 负责参数校验、默认值、envId/snowflake/envSequence 生成、hash 计算和文件落盘；
// - 负责把环境包元数据写入 browser_envs 索引表，便于列表查询、假删除和后续监控；
// - 不负责 Docker create/start，不检查端口占用，不写 manifest.lastRuntime；
// - 写入失败时会清理本次新建的环境包目录，避免留下半成品。
func (s *Service) CreateBrowserEnv(param *model.CreateBrowserEnvRequest) (*model.CreateBrowserEnvResponse, error) {
	normalized, err := normalizeCreateRequest(param)
	if err != nil {
		return nil, err
	}

	createEnvMu.Lock()
	defer createEnvMu.Unlock()

	ctx, err := newCreateContext(normalized)
	if err != nil {
		return nil, err
	}
	if err = ensureEnvPathAvailable(ctx.AbsoluteEnvPath); err != nil {
		return nil, err
	}
	if err = createEnvDirectories(ctx.AbsoluteEnvPath); err != nil {
		return nil, internalError(err.Error())
	}

	created := false
	defer cleanupPartialEnvPackage(ctx.AbsoluteEnvPath, &created)

	files, err := buildPackageFiles(ctx)
	if err != nil {
		return nil, err
	}
	if err = writeEnvPackageFiles(ctx.AbsoluteEnvPath, ctx.Paths, files); err != nil {
		return nil, internalError(err.Error())
	}
	if err = browserEnvDao.NewCreateModelHandler().CreateBrowserEnvIndex(context.Background(), buildBrowserEnvIndex(ctx, files)); err != nil {
		if errors.Is(err, browserEnvDao.ErrDuplicateBrowserEnv) {
			return nil, conflictError("envId 已存在")
		}
		return nil, internalError(err.Error())
	}

	created = true
	return buildCreateResponse(ctx), nil
}

// ListBrowserEnvs 查询本机环境包索引列表。
//
// 设计来源：
// - 用户要求边缘服务能直接返回当前管理了多少配置文件；
// - 现在 browser_envs 已作为本机索引表，列表接口应查询 SQLite，而不是每次扫描目录；
// - 默认排除 deleted，保留后续假删除、回收站和状态监控的扩展空间。
//
// 职责边界：
// - 负责参数归一化、调用 Dao 查询列表和统计、组装响应；
// - 不读取 profile/proxy/fingerprint 文件，不判断 Docker 实时状态；
// - 后续如果要补“目录是否仍存在”的一致性检查，应作为额外校验，不要改变数据库索引是列表主来源的原则。
func (s *Service) ListBrowserEnvs(query model.ListBrowserEnvQuery, httpBase string, wsBase string) (*model.ListBrowserEnvResponse, error) {
	normalized, err := normalizeListQuery(query)
	if err != nil {
		return nil, err
	}

	handler := browserEnvDao.NewListModelHandler()
	total, err := handler.CountBrowserEnvIndexes(context.Background(), normalized)
	if err != nil {
		return nil, internalError(err.Error())
	}
	byStatus, err := handler.CountBrowserEnvByStatus(context.Background(), normalized)
	if err != nil {
		return nil, internalError(err.Error())
	}
	byRPAType, err := handler.CountBrowserEnvByRPAType(context.Background(), normalized)
	if err != nil {
		return nil, internalError(err.Error())
	}
	items, err := handler.ListBrowserEnvIndexes(context.Background(), normalized)
	if err != nil {
		return nil, internalError(err.Error())
	}
	attachRunningVNCLinks(items, httpBase, wsBase)

	return &model.ListBrowserEnvResponse{
		Total:     total,
		Page:      normalized.Page,
		PageSize:  normalized.PageSize,
		ByStatus:  byStatus,
		ByRPAType: byRPAType,
		Items:     items,
	}, nil
}

// attachRunningVNCLinks 给运行中的环境包列表项补充浏览器 VNC 地址。
//
// 设计来源：
// - 用户要求 `/api/v1/edge/browser-envs?status=running` 直接返回 VNC 链接，前端列表不应再逐条调用 vnc-info；
// - VNC 链接只对 running 状态有意义，非运行态不返回这些字段，避免 UI 误导用户点击不可用连接；
// - 地址按当前请求 Host 生成，兼容本机访问和后续反向代理。
//
// 职责边界：
// - 只补充连接地址，不探测 VNC 是否健康；
// - 不读取环境包文件，不打开 Docker；
// - 目标端口仍来自 browser_envs.vnc_port，不允许前端透传。
func attachRunningVNCLinks(items []*model.BrowserEnvIndex, httpBase string, wsBase string) {
	httpBase = strings.TrimRight(strings.TrimSpace(httpBase), "/")
	wsBase = strings.TrimRight(strings.TrimSpace(wsBase), "/")
	for _, item := range items {
		if item == nil || item.Status != model.BrowserEnvStatusRunning || item.VNCPort <= 0 {
			continue
		}
		escapedEnvID := url.PathEscape(item.EnvID)
		queryEnvID := url.QueryEscape(item.EnvID)
		item.VNCURL = fmt.Sprintf("vnc://127.0.0.1:%d", item.VNCPort)
		if wsBase != "" {
			item.VNCWSURL = fmt.Sprintf("%s/api/v1/edge/browser-envs/%s/vnc/ws", wsBase, escapedEnvID)
		}
		if httpBase != "" {
			item.WebVNCURL = fmt.Sprintf("%s/web-vnc.html?envId=%s", httpBase, queryEnvID)
		}
	}
}

// buildBrowserEnvIndex 把已成功生成的环境包信息整理成数据库索引记录。
//
// 设计来源：
// - 文件系统保存完整环境包，SQLite 只保存可查询、可假删除、可监控的轻量元数据；
// - 创建阶段还没有真正启动 Docker，因此 container_status/monitor_status 不能伪造为 running；
// - fingerprint_restored 表示“已注入运行态容器”，不是“有备份文件”，所以创建时固定为 false。
func buildBrowserEnvIndex(ctx *createContext, files envPackageFiles) *model.BrowserEnvIndex {
	containerName := files.Container.ContainerName
	return &model.BrowserEnvIndex{
		EnvID:               ctx.EnvID,
		UserID:              ctx.Param.UserID,
		RPAType:             ctx.Param.RPAType,
		Name:                ctx.Param.Name,
		EnvSequence:         ctx.EnvSequence,
		CDPPort:             ctx.Ports.CDP,
		VNCPort:             ctx.Ports.VNC,
		EnvPath:             ctx.RelativeEnvPath,
		Status:              model.BrowserEnvStatusCreated,
		ContainerName:       &containerName,
		ContainerStatus:     model.BrowserEnvContainerStatusUnknown,
		MonitorStatus:       model.BrowserEnvMonitorStatusUnknown,
		FingerprintRestored: false,
		HasBrowserData:      true,
		CreatedAt:           ctx.Now,
		UpdatedAt:           ctx.Now,
	}
}

// newCreateContext 生成创建环境包所需的派生数据。
//
// 这里集中处理 snowflake、envId、端口、路径和 hash，主流程就能保持“校验 -> 生成上下文 -> 写文件”的清晰顺序。
func newCreateContext(param *model.CreateBrowserEnvRequest) (*createContext, error) {
	now := time.Now().Unix()
	snowflakeID := idGen.Next()
	envID := buildEnvID(param.UserID, param.RPAType, snowflakeID)
	paths := defaultManifestPaths()
	envSequence, err := nextEnvSequence()
	if err != nil {
		return nil, internalError(err.Error())
	}

	ctx := &createContext{
		Param:       param,
		Now:         now,
		SnowflakeID: snowflakeID,
		EnvID:       envID,
		BindingID:   fmt.Sprintf("binding-%s-%s-%s", param.UserID, param.RPAType, snowflakeID),
		EnvSequence: envSequence,
		Ports: model.BrowserEnvPorts{
			CDP: 8100 + envSequence,
			VNC: 9100 + envSequence,
		},
		Paths:           paths,
		RelativeEnvPath: filepath.ToSlash(filepath.Join("data", "browser-envs", "users", param.UserID, param.RPAType, envID)),
	}
	if Settings.Conf.ProjectRoot == "" {
		return nil, internalError("project root 不能为空")
	}
	ctx.AbsoluteEnvPath = filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(ctx.RelativeEnvPath))

	ctx.ProxyConfigHash = buildTextHash(param.Proxy.Config)
	ctx.Identity = buildBindingIdentity(param, paths, ctx.ProxyConfigHash)
	ctx.IdentityHash, err = buildJSONHash(ctx.Identity)
	if err != nil {
		return nil, internalError(fmt.Sprintf("计算 identityHash 失败: %v", err))
	}
	ctx.ConfigHash = ctx.IdentityHash
	return ctx, nil
}

// buildBindingIdentity 组装参与 identityHash 的稳定身份字段。
//
// 维护约束：
// - 这里只放 userId、rpaType、时区、语言、屏幕、代理配置 hash 和 browserData 相对路径；
// - 不允许加入 deviceId、containerId、端口、Docker API、lastRuntime 等运行位置字段。
func buildBindingIdentity(param *model.CreateBrowserEnvRequest, paths model.ManifestPaths, proxyConfigHash string) model.BindingIdentity {
	return model.BindingIdentity{
		UserID:   param.UserID,
		RPAType:  param.RPAType,
		Timezone: param.Environment.Timezone,
		Language: param.Environment.Language,
		Screen: model.BindingIdentityScreen{
			Width:  param.Environment.Screen.Width,
			Height: param.Environment.Screen.Height,
		},
		Proxy: model.BindingIdentityProxy{
			Type:       param.Proxy.Type,
			ConfigHash: proxyConfigHash,
		},
		BrowserDataPath: paths.BrowserData,
	}
}

// ensureEnvPathAvailable 防止覆盖已有环境包。
//
// envId 理论上由 snowflake 保证唯一，但文件系统仍然是最终事实来源；
// 如果目录已经存在，宁可返回冲突，也不能覆盖可能已经包含登录态的 browser-data。
func ensureEnvPathAvailable(envPath string) error {
	if _, statErr := os.Stat(envPath); statErr == nil {
		return conflictError("envPath 已存在但不是本次创建的新环境包")
	} else if !os.IsNotExist(statErr) {
		return internalError(fmt.Sprintf("检查 envPath 失败: %v", statErr))
	}
	return nil
}

func cleanupPartialEnvPackage(envPath string, created *bool) {
	if created == nil || *created {
		return
	}
	_ = os.RemoveAll(envPath)
}

func buildCreateResponse(ctx *createContext) *model.CreateBrowserEnvResponse {
	return &model.CreateBrowserEnvResponse{
		EnvID:       ctx.EnvID,
		UserID:      ctx.Param.UserID,
		RPAType:     ctx.Param.RPAType,
		EnvSequence: ctx.EnvSequence,
		Ports:       ctx.Ports,
		EnvPath:     ctx.RelativeEnvPath,
		Files: map[string]string{
			"manifest":                 "manifest.json",
			"profile":                  ctx.Paths.Profile,
			"binding":                  ctx.Paths.Binding,
			"container":                ctx.Paths.Container,
			"proxyConfig":              ctx.Paths.ProxyConfig,
			"fingerprintSnapshot":      ctx.Paths.FingerprintSnapshot,
			"fingerprintBackup":        ctx.Paths.FingerprintBackup,
			"fingerprintRuntimeConfig": ctx.Paths.FingerprintRuntimeConfig,
			"browserData":              ctx.Paths.BrowserData,
		},
		IdentityHash: ctx.IdentityHash,
		ConfigHash:   ctx.ConfigHash,
		CreatedAt:    ctx.Now,
	}
}
