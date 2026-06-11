package Routes

import "testing"

// TestBrowserEnvDeleteRoutesAreSplit 验证环境包删除相关路由的权限边界。
//
// 设计来源：
// - 用户明确要求重新开发阶段不保留旧 /delimage 或根 DELETE 冗余入口；
// - /del 只负责删除环境包关联运行镜像，/package 才负责彻底销毁环境包资产；
// - 这里直接检查 Gin 路由表，不发真实 HTTP 请求，避免测试触碰 SQLite、Docker 或 browser-data/profile。
func TestBrowserEnvDeleteRoutesAreSplit(t *testing.T) {
	engine := Setup()
	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	if !routes["DELETE /api/v1/edge/browser-envs/:envId/del"] {
		t.Fatalf("expected /del image delete route to be registered")
	}
	if !routes["DELETE /api/v1/edge/browser-envs/:envId/package"] {
		t.Fatalf("expected /package env package delete route to be registered")
	}
	if routes["DELETE /api/v1/edge/browser-envs/:envId/delimage"] {
		t.Fatalf("legacy /delimage route must not be registered")
	}
	if routes["DELETE /api/v1/edge/browser-envs/:envId"] {
		t.Fatalf("root DELETE /browser-envs/:envId must not be registered")
	}
}
