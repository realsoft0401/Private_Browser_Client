package Routes

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"private_browser_client/Pkg/HttpResponse"
	BrowserEnvService "private_browser_client/Service/BrowserEnv"
	EdgeService "private_browser_client/Service/Edge"
	HealthService "private_browser_client/Service/Health"
	NodeRegisterService "private_browser_client/Service/NodeRegister"
	SlotService "private_browser_client/Service/Slot"
	TaskService "private_browser_client/Service/Task"
	"private_browser_client/Settings"
)

// Setup 统一注册当前服务所有 HTTP 路由。
//
// 当前新 Client 先保留三类正式入口：
// 1. 工具页：`/swagger`、`/scalar`、`/openapi.yaml`
// 2. 本机事实：`/health`、`/api/v1/edge/device-info`
// 3. 后续 slot/package 执行接口预留挂载点
//
// 这里回到 old 的路由组织方式：Routes 层只负责把入口挂出来，
// 具体事实和业务仍然分到对应 Service 包。
func Setup() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/", func(c *gin.Context) {
		HttpResponse.ResponseSuccess(c, gin.H{
			"service": fmt.Sprintf("Private_Browser_Client RESTful service"),
			"version": Settings.Conf.Version,
			"mode":    Settings.Conf.Mode,
		})
	})

	r.GET("/health", func(c *gin.Context) {
		HttpResponse.ResponseSuccess(c, HealthService.BuildHealthResponse())
	})

	registerSwaggerDocs(r)

	apiV1 := r.Group("/api/v1")
	edge := apiV1.Group("/edge")
	edge.GET("/device-info", EdgeService.GetDeviceInfo)
	edge.GET("/docker/status", EdgeService.GetDockerStatus)
	edge.GET("/docker/images", EdgeService.GetDockerImages)
	edge.GET("/docker/containers", EdgeService.GetDockerContainers)
	edge.POST("/docker/pull-image", EdgeService.PullDockerImage)
	edge.POST("/docker/remove-image", EdgeService.RemoveDockerImage)
	edge.GET("/node-registration", NodeRegisterService.GetStatus)
	edge.POST("/node-registration/assign", NodeRegisterService.Assign)
	edge.POST("/node-registration/clear", NodeRegisterService.Clear)
	edge.POST("/containers/:slotId/start", SlotService.StartContainer)
	edge.POST("/containers/:slotId/stop", SlotService.StopContainer)
	edge.POST("/containers/:slotId/restart", SlotService.RestartContainer)
	edge.GET("/slots", SlotService.ListSlots)
	edge.POST("/slots", SlotService.CreateSlot)
	edge.GET("/slots/:slotId", SlotService.GetSlotByID)
	edge.GET("/slots/:slotId/vnc-info", SlotService.GetSlotVNCInfo)
	edge.GET("/slots/:slotId/cdp-info", SlotService.GetSlotCDPInfo)
	edge.GET("/slots/:slotId/vnc/ws", SlotService.ProxySlotVNC)
	edge.POST("/slots/:slotId/reinit", SlotService.ReinitSlot)
	edge.DELETE("/slots/:slotId", SlotService.DestroySlot)
	edge.GET("/browser-envs", BrowserEnvService.ListBrowserEnvs)
	edge.POST("/browser-envs", BrowserEnvService.CreateBrowserEnv)
	edge.GET("/browser-envs/:envId", BrowserEnvService.GetBrowserEnvDetail)
	edge.POST("/browser-envs/:envId/run", BrowserEnvService.Run)
	edge.POST("/browser-envs/:envId/stop", BrowserEnvService.Stop)
	edge.PATCH("/browser-envs/:envId/proxy", BrowserEnvService.UpdateProxy)
	edge.POST("/browser-envs/:envId/backup", BrowserEnvService.Backup)
	edge.POST("/browser-envs/:envId/restore", BrowserEnvService.Restore)
	edge.POST("/browser-envs/:envId/revalidate", BrowserEnvService.Revalidate)
	edge.POST("/browser-envs/import-package", BrowserEnvService.ImportPackage)
	edge.DELETE("/browser-envs/:envId/del", BrowserEnvService.DeleteImage)
	edge.DELETE("/browser-envs/:envId/package", BrowserEnvService.DeletePackage)
	edge.GET("/tasks/:taskId", TaskService.GetDetail)
	edge.GET("/tasks/:taskId/events", TaskService.SubscribeEvents)

	return r
}

func registerSwaggerDocs(r *gin.Engine) {
	registerStaticIfExists(r, "/vendor/swagger-ui", filepath.Join(Settings.Conf.ProjectRoot, "public", "vendor", "swagger-ui"))
	registerNoVNCStatic(r)
	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.File(filepath.Join(Settings.Conf.ProjectRoot, "docs", "openapi.yaml"))
	})
	r.GET("/swagger", swaggerPage)
	r.GET("/swagger/", swaggerPage)
	r.GET("/scalar", scalarPage)
	r.GET("/scalar/", scalarPage)
	r.GET("/web-vnc.html", webVNCPage)
}

func swaggerPage(c *gin.Context) {
	c.File(filepath.Join(Settings.Conf.ProjectRoot, "public", "swagger.html"))
}

// scalarPage 返回基于 Scalar 的 API Reference 页面。
//
// 设计来源：
// - 当前仓库已经把 `docs/openapi.yaml` 收口成唯一协议事实源；
// - 用户希望后续尝试更企业级的 API 展示方式，而不只是 Swagger UI；
// - 因此这里先保留一个极薄的 Scalar 展示页，让同一份 OpenAPI 可以同时服务 Swagger 调试页和更正式的参考文档页。
//
// 职责边界：
// - 这里只负责返回静态页面；
// - 具体协议内容仍然来自 `/openapi.yaml`；
// - 不在这里复制第二份接口定义，避免文档事实源分裂。
func scalarPage(c *gin.Context) {
	c.File(filepath.Join(Settings.Conf.ProjectRoot, "public", "scalar.html"))
}

func webVNCPage(c *gin.Context) {
	c.File(filepath.Join(Settings.Conf.ProjectRoot, "public", "web-vnc.html"))
}

func registerStaticIfExists(r *gin.Engine, path string, localDir string) {
	if stat, err := os.Stat(localDir); err == nil && stat.IsDir() {
		r.Static(path, localDir)
	}
}

func registerNoVNCStatic(r *gin.Engine) {
	candidates := []string{
		filepath.Join(Settings.Conf.ProjectRoot, "public", "vendor", "novnc"),
		filepath.Clean(filepath.Join(Settings.Conf.ProjectRoot, "..", "Private_Browser_Client_Old", "public", "vendor", "novnc")),
	}
	for _, dir := range candidates {
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			r.Static("/vendor/novnc", dir)
			return
		}
	}
}
