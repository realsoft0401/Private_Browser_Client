package Infrastructures

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"private_browser_client/Routes"
	SQLiteInfra "private_browser_client/Infrastructures/SQLite"
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
// 这里按 old 的总装方式来：
// - 先初始化配置
// - 再整理 HTTP Server 参数
// - 再创建并启动路由服务
// - 最后统一处理优雅退出
//
// 当前阶段故意不把业务逻辑塞进来，保持它只是启动总入口。
func Init(projectRoot string) error {
	if err := initDependencies(projectRoot); err != nil {
		return err
	}

	options := buildServerOptions()
	server := newHTTPServer(options)
	DiscoveryService.StartBroadcaster()
	DiscoveryService.StartHeartbeatPusher()
	startHTTPServer(server, options)
	waitForShutdownSignal()
	return shutdownHTTPServer(server)
}

func initDependencies(projectRoot string) error {
	if err := Settings.Init(projectRoot); err != nil {
		return fmt.Errorf("init setting failed: %w", err)
	}
	if err := SQLiteInfra.Init(); err != nil {
		return fmt.Errorf("init sqlite failed: %w", err)
	}
	return nil
}

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

func newHTTPServer(options serverOptions) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf("%s:%d", options.host, options.port),
		Handler:           Routes.Setup(),
		ReadHeaderTimeout: options.readTimeout,
		WriteTimeout:      options.writeTimeout,
	}
}

func startHTTPServer(server *http.Server, options serverOptions) {
	go func() {
		log.Printf("Private_Browser_Client RESTful service listening on http://%s:%d\n", options.host, options.port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen failed: %v", err)
		}
	}()
}

func waitForShutdownSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

func shutdownHTTPServer(server *http.Server) error {
	DiscoveryService.StopHeartbeatPusher()
	DiscoveryService.StopBroadcaster()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server failed: %w", err)
	}
	if err := SQLiteInfra.Close(); err != nil {
		return fmt.Errorf("close sqlite failed: %w", err)
	}
	return nil
}
