package SQLite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	"private_browser_client/Settings"
)

var DB *sql.DB

// Init 初始化边缘服务本地 SQLite 数据库。
//
// 设计来源：
// - 用户确认 browser-envs 不能只靠扫描文件夹，因为后续需要容器状态、监控状态和服务端上报；
// - SQLite 在当前阶段只作为本机边缘服务索引库，不承担中心用户、节点归属、JWT 等服务端职责；
// - 数据库文件放在项目 data 目录下，和环境包目录同级，便于本机部署、备份和排障。
//
// 职责边界：
// - 只负责打开数据库、设置连接参数、执行本机表结构迁移；
// - 不负责具体业务写入，也不把数据库句柄传到 HTTP 或 Service 里直接使用；
// - 后续如果切 MySQL 或中心库，应优先替换 Repository/基础设施层，不要把 SQL 扩散到业务代码。
func Init() error {
	if Settings.Conf.ProjectRoot == "" {
		return fmt.Errorf("project root 不能为空")
	}
	dataDir := filepath.Join(Settings.Conf.ProjectRoot, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir failed: %w", err)
	}

	dbPath := filepath.Join(dataDir, fmt.Sprintf("private_browser_client-%s.db", Settings.Conf.Env))
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open sqlite failed: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err = db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping sqlite failed: %w", err)
	}
	DB = db

	if err = migrate(); err != nil {
		_ = db.Close()
		DB = nil
		return err
	}
	return nil
}

// Close 关闭 SQLite 连接。
//
// 这里保持一个独立入口，是为了让 Infrastructures.Init 在服务退出时统一收尾；
// 后续如果加入后台监控任务，也应先停任务再关库，避免任务退出时继续写入已关闭连接。
func Close() error {
	if DB == nil {
		return nil
	}
	err := DB.Close()
	DB = nil
	return err
}

// migrate 执行当前边缘服务的最小表结构迁移。
//
// SQLite 没有稳定的字段注释元数据能力，所以建表注释必须留在源码和 project.md 中。
// browser_envs 的每个字段含义如下：
// - env_id：环境包唯一编号，由 userId + rpaType + snowflake 组成，是文件夹和数据库共同主键；
// - user_id / rpa_type：用于按用户和平台类型查询本机环境包，不表示本服务拥有中心用户系统；
// - name：展示名称，只用于本机管理和排障；
// - env_sequence：本机递增序号，是 cdp/vnc 端口规则来源，迁移到别的设备后允许重排；
// - cdp_port / vnc_port：本机端口索引，第一版按 8100/9100 + envSequence 生成；
// - env_path：环境包相对路径，数据库只存索引，不保存 profile、代理 YAML、指纹原文和浏览器数据；
// - status：环境包生命周期状态，支持 created/running/stopped/deleted/archived/error；
// - container_*：最近一次 Docker 容器运行快照，真实容器状态仍以 Docker 为最终来源；
// - monitor_status / last_error：后续本机监控与上报使用，不在创建环境包时伪造运行状态；
// - fingerprint_restored：指纹是否已注入到运行态容器，不等同于是否存在指纹备份；
// - has_browser_data：browser-data/profile 目录是否已建立，用于快速判断环境包结构是否完整；
// - *_at：生命周期时间戳，deleted_at 保留给历史假删除/归档兼容；当前 DELETE 已调整为物理删除目录并移除索引。
//
// 维护原则：
// - 这张表只做本机环境包索引和状态，不保存敏感大字段；
// - 新增状态字段时要同步更新 project.md、OpenAPI 和 Service 写入逻辑；
// - 物理删除只能走 DELETE /browser-envs/:envId，并必须先做运行态和 env_path 安全校验。
func migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS browser_envs (
			env_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			rpa_type TEXT NOT NULL,
			name TEXT NOT NULL,
			env_sequence INTEGER NOT NULL,
			cdp_port INTEGER NOT NULL,
			vnc_port INTEGER NOT NULL,
			env_path TEXT NOT NULL,
			status TEXT NOT NULL,
			container_id TEXT,
			container_name TEXT,
			container_status TEXT NOT NULL DEFAULT 'unknown',
			monitor_status TEXT NOT NULL DEFAULT 'unknown',
			last_error TEXT,
			fingerprint_restored INTEGER NOT NULL DEFAULT 0,
			has_browser_data INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER,
			last_started_at INTEGER,
			last_stopped_at INTEGER,
			last_checked_at INTEGER
		)`,
		`CREATE INDEX IF NOT EXISTS idx_browser_envs_status ON browser_envs(status)`,
		`CREATE INDEX IF NOT EXISTS idx_browser_envs_user_rpa ON browser_envs(user_id, rpa_type)`,
		`CREATE INDEX IF NOT EXISTS idx_browser_envs_sequence ON browser_envs(env_sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_browser_envs_updated_at ON browser_envs(updated_at)`,
	}

	for _, statement := range statements {
		if _, err := DB.Exec(statement); err != nil {
			return fmt.Errorf("migrate sqlite failed: %w", err)
		}
	}
	return nil
}
