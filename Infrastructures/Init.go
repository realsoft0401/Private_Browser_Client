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
	DiscoveryService "private_browser_client/Service/Discovery"
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
	DiscoveryService.StartBroadcaster()
	defer func() {
		DiscoveryService.StopBroadcaster()
		BrowserEnvService.StopStatusSyncManager()
		if err := SQLite.Close(); err != nil {
			log.Printf("close sqlite failed: %v\n", err)
		}
	}()
	options := buildServerOptions()

	// Edge Client 后期没有开发/测试/生产运行模式，所以启动阶段不能再为了本地便利自动 kill 旧进程。
	// 端口被占用时直接失败，并给出 lsof/kill 排查方向，由管理员或测试人员明确处理。
	if err := ensurePortAvailable(options.port); err != nil {
		return err
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

// ensurePortAvailable 按端口查找当前 LISTEN 进程，并在存在占用时明确失败。
//
// 设计来源：
// - 用户确认 Edge Client 后期全部按生产口径运行，不再区分开发/测试/生产模式；
// - 因此服务启动不能因为本地测试方便而自动 kill 其它进程，避免商业节点误伤同端口服务；
// - 这里只做可执行的错误提示：指出哪个端口被哪些 PID 占用，并给出管理员排查命令。
func ensurePortAvailable(port int) error {
	// 这里只检查真正处于 LISTEN 的服务进程。
	//
	// 历史问题：早期使用 `lsof -ti tcp:端口` 会把“连接过这个端口的客户端进程”也列出来，
	// 例如 Codex 内置网络进程访问过 3300 后也可能被误判为占用者。
	// 当前边缘服务只关心是否已有监听进程，所以必须加 `-sTCP:LISTEN` 收紧范围。
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
	occupied := make([]string, 0, len(pids))
	for _, pid := range pids {
		if pid == "" || pid == currentPID {
			continue
		}
		occupied = append(occupied, pid)
	}
	if len(occupied) > 0 {
		return fmt.Errorf(
			"端口 %d 已被进程占用，Client 不能启动；影响范围：http 服务无法监听 3300，Swagger/health/edge API 都不可用；解决方式：先执行 `lsof -nP -iTCP:%d -sTCP:LISTEN` 确认 PID，再由管理员明确停止旧服务，例如 `kill <pid>` 或 `docker rm -f <container>`；本服务不会自动 kill 进程，避免误伤商业节点。占用 PID=%s",
			port,
			port,
			strings.Join(occupied, ","),
		)
	}
	return nil
}
