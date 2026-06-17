package Common

import (
	"database/sql"

	sqliteInfra "private_browser_client/Infrastructures/SQLite"
)

// DBProvider 是 Repository 层获取数据库连接的最小抽象。
//
// 当前新 Client 还没有把 SQLite 真正接起来，所以这里先保留一个很薄的 provider 入口。
// 后续接入 Infrastructures/SQLite 后，应继续通过这一层拿连接，
// 不要让每个 Repository 自己去引用基础设施全局变量。
type DBProvider interface {
	DB() *sql.DB
}

// DB 返回当前 Client 的本地 SQLite 连接。
//
// 设计来源：
// - old 项目要求 Repository 统一通过基础设施拿连接，不要各层自己 open sqlite；
// - 新 Client 这次把 SQLite 正式接回后，也继续保持这条边界，避免 Repository 直接依赖配置或文件路径；
// - 这样后续如果要做连接监控、只读模式或替换存储层，修改点仍然集中在基础设施。
func DB() *sql.DB {
	return sqliteInfra.DB()
}
