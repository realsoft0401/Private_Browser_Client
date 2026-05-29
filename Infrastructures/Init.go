package Infrastructures

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"private_browser_client/Infrastructures/SQLite"
	"private_browser_client/Routes"
	BrowserEnvService "private_browser_client/Service/BrowserEnv"
	"private_browser_client/Settings"
)

type serverOptions struct {
	host         string
	port         int
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// Init 统一完成基础设施初始化和 HTTP 服务启动。
//
// 这个函数现在是服务端的总装入口：
// - 它负责把配置、路由、HTTP Server 和优雅退出串起来；
// - 它不承载具体业务规则，业务仍然应该落在 Service / Dao 层；
// - 后续如果要补日志、任务队列、定时任务，也建议继续在这里按阶段插入，不要让 main.go 膨胀回黑盒入口。
func Init(projectRoot string) error {
	if err := initDependencies(projectRoot); err != nil {
		return err
	}
	BrowserEnvService.StartStatusSyncManager()
	defer func() {
		BrowserEnvService.StopStatusSyncManager()
		if err := SQLite.Close(); err != nil {
			log.Printf("close sqlite failed: %v\n", err)
		}
	}()
	options := buildServerOptions()

	// 这是用户明确提出的开发期约束：每次重新启动服务前，都先按监听端口执行一次 `lsof`，
	// 如果发现旧进程还占着端口，就直接杀掉，避免开发时反复手工查 PID 再 kill。
	// 这里的职责是“清理当前端口占用”，不负责做通用进程管理；后续如果进入生产环境，
	// 应优先把这段逻辑收敛到开发模式开关内，不要无差别地对所有环境都强制 kill 端口占用进程。
	if err := releaseOccupiedPort(options.port); err != nil {
		return fmt.Errorf("release occupied port failed: %w", err)
	}

	server := newHTTPServer(options)
	startHTTPServer(server, options)
	waitForShutdownSignal()
	return shutdownHTTPServer(server)
}

// initDependencies 统一初始化边缘服务运行依赖。
//
// 这里单独拆出来，是为了让 `Init` 主流程只保留“先准备依赖，再启动服务”这类大步骤，
// 避免配置、路由、进程控制全堆在一个函数里读不清。
func initDependencies(projectRoot string) error {
	if err := Settings.Init(projectRoot); err != nil {
		return fmt.Errorf("init setting failed: %w", err)
	}
	if err := SQLite.Init(); err != nil {
		return fmt.Errorf("init sqlite failed: %w", err)
	}
	return nil
}

// buildServerOptions 把配置里的监听参数整理成一个清晰的运行时结构。
//
// 这样做的原因不是为了“形式化”，而是让 host、port、timeout 这类启动参数先集中收口，
// 后面构建 server 时只读一个对象，主流程更直观。
func buildServerOptions() serverOptions {
	options := serverOptions{
		host:         Settings.Conf.ServerConfig.Host,
		port:         Settings.Conf.ServerConfig.Port,
		readTimeout:  time.Duration(Settings.Conf.ServerConfig.ReadTimeoutSeconds) * time.Second,
		writeTimeout: time.Duration(Settings.Conf.ServerConfig.WriteTimeoutSeconds) * time.Second,
	}
	if options.host == "" {
		options.host = "0.0.0.0"
	}
	if options.port <= 0 {
		options.port = 3300
	}
	if options.readTimeout <= 0 {
		options.readTimeout = 15 * time.Second
	}
	if options.writeTimeout <= 0 {
		options.writeTimeout = 15 * time.Second
	}
	return options
}

// newHTTPServer 根据整理后的启动参数创建标准 HTTP Server。
//
// 路由构建和服务实例化放在这里统一完成，后续如果要补 header timeout、
// idle timeout 或中间件包装，也只需要看这一处。
func newHTTPServer(options serverOptions) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf("%s:%d", options.host, options.port),
		Handler:           Routes.Setup(),
		ReadHeaderTimeout: options.readTimeout,
		WriteTimeout:      options.writeTimeout,
	}
}

// startHTTPServer 异步启动 HTTP 服务。
//
// 它保持单一职责：只启动，不处理退出信号。
// 这样主流程就能明显看出“启动服务”和“等待退出”是两个步骤，而不是混在一起。
func startHTTPServer(server *http.Server, options serverOptions) {
	go func() {
		log.Printf("Private_Browser_Client RESTful service listening on http://%s:%d\n", options.host, options.port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen failed: %v", err)
		}
	}()
}

// waitForShutdownSignal 阻塞等待标准退出信号。
//
// 当前只监听 `SIGINT` 和 `SIGTERM`，保持行为简单可预期。
// 后续如果真要接热更新或守护控制，再在这里统一扩展，不要在别的包里各自监听一次。
func waitForShutdownSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

// shutdownHTTPServer 执行优雅关机。
//
// 关闭阶段仍保留 5 秒窗口，让正在处理的请求尽量收干净。
// 这里独立出来后，`Init` 最后一步一眼就能看出是在“优雅关闭服务”。
func shutdownHTTPServer(server *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server failed: %w", err)
	}
	return nil
}

// releaseOccupiedPort 按端口查找当前监听进程，并在存在占用时直接结束它。
//
// 这段实现的来历不是通用最佳实践，而是当前项目的开发约定：
// - 用户已经明确要求“每次关闭后，重新启动前都按端口 lsof 一次，再 kill”；
// - 所以这里直接复用系统命令，把原本手工执行的排障动作前置为启动前自清理；
// - 后续如果要细分环境，至少要保留这个需求在 dev 场景成立，不要又退回手工查端口的旧流程。
func releaseOccupiedPort(port int) error {
	// 这里只释放真正处于 LISTEN 的服务进程。
	//
	// 历史问题：早期使用 `lsof -ti tcp:端口` 会把“连接过这个端口的客户端进程”也列出来，
	// 例如 Codex 内置网络进程访问过 3300 后也可能被误判为占用者，导致启动阶段尝试 kill 无关进程。
	// 当前边缘服务只需要清理旧的监听服务，所以必须加 `-sTCP:LISTEN` 收紧范围。
	listCmd := exec.Command("lsof", "-tiTCP:"+fmt.Sprintf("%d", port), "-sTCP:LISTEN")
	output, err := listCmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("lsof query failed: %w", err)
	}

	pids := strings.Fields(strings.TrimSpace(string(output)))
	currentPID := fmt.Sprintf("%d", os.Getpid())
	for _, pid := range pids {
		if pid == "" || pid == currentPID {
			continue
		}
		killCmd := exec.Command("kill", "-9", pid)
		if killErr := killCmd.Run(); killErr != nil {
			return fmt.Errorf("kill pid %s failed: %w", pid, killErr)
		}
		log.Printf("killed process on port %d, pid=%s\n", port, pid)
	}
	return nil
}
