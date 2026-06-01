package Routes

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	BrowserEnvService "private_browser_client/Service/BrowserEnv"
	EdgeService "private_browser_client/Service/Edge"
	TaskService "private_browser_client/Service/Task"
	"private_browser_client/Settings"
)

// Setup 统一注册当前服务所有 HTTP 路由。
//
// 当前 Client 已收紧为边缘服务，因此这里只注册本机能力接口。
// 用户、节点列表、多节点调度等中心服务端接口不要再加回这里，应放到未来 Private_Browser_Server。
func Setup() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, fmt.Sprintf("Private_Browser_Client RESTful service\nversion=%s\nmode=%s\n", Settings.Conf.Version, Settings.Conf.Mode))
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"ok":         true,
			"service":    Settings.Conf.Name,
			"mode":       Settings.Conf.Mode,
			"version":    Settings.Conf.Version,
			"configFile": Settings.Conf.ConfigFile,
			"dockerApi":  Settings.Conf.DockerConfig.APIURL,
			"statusSync": BrowserEnvService.StatusSyncSnapshot(),
		})
	})
	registerSwaggerDocs(r)
	r.GET("/web-vnc.html", BrowserEnvService.WebVNCPage)
	registerNoVNCStatic(r)

	apiV1 := r.Group("/api/v1")
	edge := apiV1.Group("/edge")

	// Edge 域只暴露本机能力，不带 nodeId，也不访问中心节点表。
	edge.GET("/device-info", EdgeService.GetDeviceInfo)
	edge.GET("/docker/status", EdgeService.GetDockerStatus)
	edge.GET("/docker/images", EdgeService.GetDockerImages)
	edge.GET("/docker/containers", EdgeService.GetDockerContainers)
	edge.POST("/docker/pull-image", EdgeService.PullDockerImage)
	edge.POST("/docker/remove-image", EdgeService.RemoveDockerImage)
	edge.POST("/containers/:id/start", EdgeService.StartDockerContainer)
	edge.POST("/containers/:id/stop", EdgeService.StopDockerContainer)
	edge.POST("/containers/:id/restart", EdgeService.RestartDockerContainer)
	edge.GET("/tasks/:taskId", TaskService.GetEdgeTask)
	edge.GET("/tasks/:taskId/events", TaskService.StreamEdgeTaskEvents)
	edge.GET("/browser-envs", BrowserEnvService.ListBrowserEnvs)
	edge.POST("/browser-envs", BrowserEnvService.CreateBrowserEnv)
	edge.POST("/browser-envs/import-package", BrowserEnvService.ImportBrowserEnvPackage)
	edge.GET("/browser-envs/:envId", BrowserEnvService.GetBrowserEnvDetail)
	edge.POST("/browser-envs/:envId/run", BrowserEnvService.RunBrowserEnv)
	edge.POST("/browser-envs/:envId/stop", BrowserEnvService.StopBrowserEnv)
	edge.POST("/browser-envs/:envId/backup-package", BrowserEnvService.BackupBrowserEnvPackage)
	edge.POST("/browser-envs/:envId/export-and-remove", BrowserEnvService.ExportAndRemoveBrowserEnvPackage)
	edge.DELETE("/browser-envs/:envId", BrowserEnvService.DeleteBrowserEnv)
	edge.PATCH("/browser-envs/:envId/proxy", BrowserEnvService.UpdateBrowserEnvProxy)
	edge.PATCH("/browser-envs/:envId/proxy-mode", BrowserEnvService.UpdateBrowserEnvProxyMode)
	edge.GET("/browser-envs/:envId/cdp-test", BrowserEnvService.TestBrowserEnvCDP)
	edge.GET("/browser-envs/:envId/vnc-info", BrowserEnvService.GetBrowserEnvVNCInfo)
	edge.GET("/browser-envs/:envId/vnc/ws", BrowserEnvService.ProxyBrowserEnvVNC)

	return r
}

// registerSwaggerDocs 挂载 OpenAPI 和 Swagger UI。
//
// 设计来源：
// - 用户已经用 Apifox 测试接口，现在又要求 Dockerfile 顺便“装上 swagger”；
// - OpenAPI 主事实仍然是 docs/openapi.yaml，Swagger UI 只是容器部署后的浏览器查看入口；
// - 这里不引入后端动态生成文档，避免接口文档和用户参与确认过的 openapi.yaml 分裂成两套来源。
func registerSwaggerDocs(r *gin.Engine) {
	swaggerUIDir := filepath.Join(Settings.Conf.ProjectRoot, "public", "vendor", "swagger-ui")
	if stat, err := os.Stat(swaggerUIDir); err == nil && stat.IsDir() {
		r.Static("/vendor/swagger-ui", swaggerUIDir)
	}
	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.File(filepath.Join(Settings.Conf.ProjectRoot, "docs", "openapi.yaml"))
	})
	r.GET("/swagger", swaggerPage)
	r.GET("/swagger/", swaggerPage)
}

// swaggerPage 返回 Swagger UI 页面。
//
// 当前页面是很薄的一层静态壳，实际接口定义从 /openapi.yaml 读取；
// 后续新增接口时只要维护 docs/openapi.yaml，Docker 中的 Swagger UI 会自动展示最新文档。
func swaggerPage(c *gin.Context) {
	c.File(filepath.Join(Settings.Conf.ProjectRoot, "public", "swagger.html"))
}

// registerNoVNCStatic 挂载 noVNC 前端资源。
//
// 设计来源：
// - 用户在 Mac 上使用原生 VNC 会遇到密码弹窗，因此当前阶段新增浏览器 noVNC 页面；
// - 但用户之前明确不想恢复旧静态控制台，所以这里只挂 noVNC 必要 vendor 目录，不引入整套 public/app.js 控制台；
// - 当前复用 Private_Browser_Control 已安装的 @novnc/novnc，后续打包 Client 时应把该目录复制到 Client 自己的 public/vendor 下。
func registerNoVNCStatic(r *gin.Engine) {
	candidates := []string{
		filepath.Join(Settings.Conf.ProjectRoot, "public", "vendor", "novnc"),
		filepath.Clean(filepath.Join(Settings.Conf.ProjectRoot, "..", "Private_Browser_Control", "control-api", "node_modules", "@novnc", "novnc")),
	}
	for _, dir := range candidates {
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			r.Static("/vendor/novnc", dir)
			return
		}
	}
}
