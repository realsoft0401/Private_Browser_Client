package SQLite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	"private_browser_client/Settings"
)

var db *sql.DB

// Init 初始化新 Client 的本地 SQLite。
//
// 设计来源：
//   - old Client 里 SQLite 是本机资产索引事实源，新 Client 当前虽然已经把 slot/package/runtime relation
//     和 node registration 功能做起来了，但如果没有 SQLite，重启后当前态会全部丢失；
//   - 用户刚刚明确指出这层不能缺，因此这里把 new 模型对应的本机事实重新收回数据库；
//   - 仍然坚持 Client 只保存本机索引和当前态，不保存中心用户体系、多节点列表或中心任务历史。
//
// 职责边界：
// - 只负责本机数据库文件、连接参数和表结构迁移；
// - 不负责业务读写，不在这里拼 slot/package/run 业务逻辑；
// - 后续如果替换底层存储，也应优先替换基础设施和 Repository，而不是把 SQL 撒进 Service。
func Init() error {
	if Settings.Conf.ProjectRoot == "" {
		return fmt.Errorf("project root 不能为空")
	}
	dataDir := filepath.Join(Settings.Conf.ProjectRoot, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir failed: %w", err)
	}

	dbPath := filepath.Join(dataDir, "private_browser_client.db")
	conn, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open sqlite failed: %w", err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	if err = conn.Ping(); err != nil {
		_ = conn.Close()
		return fmt.Errorf("ping sqlite failed: %w", err)
	}
	db = conn
	if err = migrate(); err != nil {
		_ = conn.Close()
		db = nil
		return err
	}
	return nil
}

// DB 返回当前 SQLite 连接。
func DB() *sql.DB {
	return db
}

// Close 在服务退出时关闭 SQLite。
func Close() error {
	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	return err
}

// migrate 创建新 Client 现阶段真正需要的本机事实表。
//
// 表职责：
// - browser_envs：正式环境包资产索引，只保存列表、状态和运行摘要；
// - slots：资源位当前态，不保存长历史；
// - package_runtime_views：package 在本机的当前运行视图，不替代长期包资产；
// - runtime_relations：当前 package <-> slot 运行关系。
//
// 不能退回的原则：
// - Client SQLite 只保存本机 browser-env/slot/package/runtime 事实；
// - Node Server 分配的中心 clientId 不属于 Edge 本机事实，不能继续落到本地库。
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
			container_status TEXT NOT NULL DEFAULT 'missing',
			monitor_status TEXT NOT NULL DEFAULT 'unknown',
			last_error TEXT,
			backup_path TEXT,
			backup_checksum TEXT,
			backup_size INTEGER,
			backup_at INTEGER,
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
		`CREATE TABLE IF NOT EXISTS slots (
			slot_id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			current_package_id TEXT,
			current_run_id TEXT,
			container_id TEXT,
			container_name TEXT,
			runtime_image TEXT,
			container_status TEXT,
			cdp_port INTEGER,
			vnc_port INTEGER,
			last_error TEXT,
			initialized_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_slots_status ON slots(status)`,
		`CREATE TABLE IF NOT EXISTS package_runtime_views (
			package_id TEXT PRIMARY KEY,
			current_run_id TEXT,
			current_slot_id TEXT,
			runtime_status TEXT NOT NULL,
			last_run_at INTEGER,
			last_stop_at INTEGER,
			last_error TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_package_runtime_status ON package_runtime_views(runtime_status)`,
		`CREATE TABLE IF NOT EXISTS runtime_relations (
			run_id TEXT PRIMARY KEY,
			package_id TEXT NOT NULL UNIQUE,
			slot_id TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL,
			last_error TEXT,
			started_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_relations_status ON runtime_relations(status)`,
		`DROP TABLE IF EXISTS node_registration`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("migrate sqlite failed: %w", err)
		}
	}
	if err := ensureBrowserEnvColumns(); err != nil {
		return err
	}
	return nil
}

// ensureBrowserEnvColumns 给已存在的旧表补齐后续新增字段。
//
// 这次主要补 backup 相关字段，避免旧本地库在升级后 restore/backup 无法落库。
func ensureBrowserEnvColumns() error {
	columns := map[string]string{
		"backup_path":     "TEXT",
		"backup_checksum": "TEXT",
		"backup_size":     "INTEGER",
		"backup_at":       "INTEGER",
	}
	for name, definition := range columns {
		exists, err := sqliteColumnExists("browser_envs", name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err = db.Exec(fmt.Sprintf("ALTER TABLE browser_envs ADD COLUMN %s %s", name, definition)); err != nil {
			return fmt.Errorf("add browser_envs.%s failed: %w", name, err)
		}
	}
	return nil
}

func sqliteColumnExists(table string, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("read sqlite table info failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue any
		var pk int
		if err = rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan sqlite table info failed: %w", err)
		}
		if name == column {
			return true, nil
		}
	}
	if err = rows.Err(); err != nil {
		return false, fmt.Errorf("iterate sqlite table info failed: %w", err)
	}
	return false, nil
}
