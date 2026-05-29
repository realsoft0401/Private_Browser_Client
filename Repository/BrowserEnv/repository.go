package BrowserEnv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mattn/go-sqlite3"

	"private_browser_client/Infrastructures/SQLite"
	model "private_browser_client/Models/BrowserEnv"
)

var ErrDuplicateBrowserEnv = errors.New("browser env already exists")
var ErrBrowserEnvNotFound = errors.New("browser env not found")

// Repository 封装 browser_envs 表的最底层数据库访问。
//
// 设计来源：
// - 用户要求保留旧框架里“业务动作入口清晰”的 Dao 风格，同时补一层 Repository；
// - Repository 只处理 SQL、RowsAffected 和数据库错误归一化，不写 HTTP、不写中文业务提示；
// - 这样后续 SQLite 切换为 MySQL 时，优先改这一层，不让 Service 和 Dao 到处散写 SQL。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建 browser_envs 仓储。
//
// 当前仓储使用基础设施层已经初始化好的 SQLite 连接；
// 如果后续做单元测试，可以在这里扩展为可注入连接，而不是让业务层直接接触全局 DB。
func NewRepository() *Repository {
	return &Repository{db: SQLite.DB}
}

// InsertBrowserEnvIndex 插入一条环境包索引记录。
//
// 职责边界：
// - 只把已由 Service/Dao 整理好的字段落库；
// - 不生成 envId、不计算端口、不判断环境包文件是否完整；
// - 主键冲突会归一化为 ErrDuplicateBrowserEnv，供上层转换成业务冲突。
func (r *Repository) InsertBrowserEnvIndex(ctx context.Context, record *model.BrowserEnvIndex) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("sqlite 未初始化")
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO browser_envs (
		env_id,
		user_id,
		rpa_type,
		name,
		env_sequence,
		cdp_port,
		vnc_port,
		env_path,
		status,
		container_id,
		container_name,
		container_status,
		monitor_status,
		last_error,
		fingerprint_restored,
		has_browser_data,
		created_at,
		updated_at,
		deleted_at,
		last_started_at,
		last_stopped_at,
		last_checked_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.EnvID,
		record.UserID,
		record.RPAType,
		record.Name,
		record.EnvSequence,
		record.CDPPort,
		record.VNCPort,
		record.EnvPath,
		record.Status,
		record.ContainerID,
		record.ContainerName,
		record.ContainerStatus,
		record.MonitorStatus,
		record.LastError,
		boolToSQLiteInt(record.FingerprintRestored),
		boolToSQLiteInt(record.HasBrowserData),
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
		record.LastStartedAt,
		record.LastStoppedAt,
		record.LastCheckedAt,
	)
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.Code == sqlite3.ErrConstraint {
			return ErrDuplicateBrowserEnv
		}
		return fmt.Errorf("insert browser_envs failed: %w", err)
	}
	return nil
}

// GetBrowserEnvIndexByID 按 envId 查询单个环境包索引。
//
// run/soft-delete/status-refresh 这类动作都必须先确认数据库索引存在；
// 这里不扫描目录兜底，是为了保持 SQLite 是列表和状态的主来源。
func (r *Repository) GetBrowserEnvIndexByID(ctx context.Context, envID string) (*model.BrowserEnvIndex, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("sqlite 未初始化")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT
		env_id,
		user_id,
		rpa_type,
		name,
		env_sequence,
		cdp_port,
		vnc_port,
		env_path,
		status,
		container_id,
		container_name,
		container_status,
		monitor_status,
		last_error,
		fingerprint_restored,
		has_browser_data,
		created_at,
		updated_at,
		deleted_at,
		last_started_at,
		last_stopped_at,
		last_checked_at
		FROM browser_envs
		WHERE env_id = ?
		LIMIT 1`, envID)
	if err != nil {
		return nil, fmt.Errorf("query browser_envs by env_id failed: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, ErrBrowserEnvNotFound
	}
	item, err := scanBrowserEnvIndex(rows)
	if err != nil {
		return nil, err
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate browser_envs by env_id failed: %w", err)
	}
	return item, nil
}

// UpdateBrowserEnvRuntime 更新环境包运行态摘要字段。
//
// 这张表只保存列表和监控需要的摘要；完整容器参数、代理配置和指纹原文仍然保存在环境包文件里。
func (r *Repository) UpdateBrowserEnvRuntime(ctx context.Context, update *model.BrowserEnvRuntimeUpdate) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("sqlite 未初始化")
	}
	if update == nil {
		return fmt.Errorf("runtime update 不能为空")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE browser_envs SET
		status = ?,
		container_id = ?,
		container_name = ?,
		container_status = ?,
		monitor_status = ?,
		last_error = ?,
		updated_at = ?,
		last_started_at = ?,
		last_stopped_at = ?,
		last_checked_at = ?
		WHERE env_id = ?`,
		update.Status,
		update.ContainerID,
		update.ContainerName,
		update.ContainerStatus,
		update.MonitorStatus,
		update.LastError,
		update.UpdatedAt,
		update.LastStartedAt,
		update.LastStoppedAt,
		update.LastCheckedAt,
		update.EnvID,
	)
	if err != nil {
		return fmt.Errorf("update browser_envs runtime failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read browser_envs update rows affected failed: %w", err)
	}
	if affected == 0 {
		return ErrBrowserEnvNotFound
	}
	return nil
}

// UpdateBrowserEnvConfig 更新配置修改后 browser_envs 需要同步的轻量字段。
//
// 职责边界：
// - 只更新 status / last_error / updated_at 这类列表展示字段；
// - 不写 profile/proxy/binding 文件，不计算 identityHash；
// - 不修改 container_id、端口和运行时间，避免配置修改污染运行态事实。
func (r *Repository) UpdateBrowserEnvConfig(ctx context.Context, update *model.BrowserEnvConfigUpdate) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("sqlite 未初始化")
	}
	if update == nil {
		return fmt.Errorf("config update 不能为空")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE browser_envs SET
		status = ?,
		last_error = ?,
		updated_at = ?
		WHERE env_id = ?`,
		update.Status,
		update.LastError,
		update.UpdatedAt,
		update.EnvID,
	)
	if err != nil {
		return fmt.Errorf("update browser_envs config failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read browser_envs config rows affected failed: %w", err)
	}
	if affected == 0 {
		return ErrBrowserEnvNotFound
	}
	return nil
}

// ListBrowserEnvIndexes 分页查询本机环境包索引列表。
//
// 职责边界：
// - 只根据 Service 已经归一化的 query 拼接安全的参数化 SQL；
// - 不读取环境包目录，不打开 profile/proxy/fingerprint 文件；
// - 默认排除 deleted 的规则由 buildBrowserEnvWhere 统一表达，避免列表和统计口径不一致。
func (r *Repository) ListBrowserEnvIndexes(ctx context.Context, query model.ListBrowserEnvQuery) ([]*model.BrowserEnvIndex, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("sqlite 未初始化")
	}
	whereSQL, args := buildBrowserEnvWhere(query)
	offset := (query.Page - 1) * query.PageSize
	args = append(args, query.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, `SELECT
		env_id,
		user_id,
		rpa_type,
		name,
		env_sequence,
		cdp_port,
		vnc_port,
		env_path,
		status,
		container_id,
		container_name,
		container_status,
		monitor_status,
		last_error,
		fingerprint_restored,
		has_browser_data,
		created_at,
		updated_at,
		deleted_at,
		last_started_at,
		last_stopped_at,
		last_checked_at
		FROM browser_envs `+whereSQL+`
		ORDER BY env_sequence ASC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("query browser_envs failed: %w", err)
	}
	defer rows.Close()

	items := make([]*model.BrowserEnvIndex, 0)
	for rows.Next() {
		item, scanErr := scanBrowserEnvIndex(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate browser_envs failed: %w", err)
	}
	return items, nil
}

// ListBrowserEnvStatusSyncTargets 查询后台状态同步任务需要扫描的环境包。
//
// 设计来源：
// - 状态同步任务要定期修正 SQLite 中的运行态，但不应处理已假删除或归档的环境包；
// - 这里不复用分页列表接口，是为了避免同步任务受 page/pageSize 影响漏扫；
// - Repository 只负责 SQL 过滤和扫描，不访问 Docker、不读取环境包文件。
func (r *Repository) ListBrowserEnvStatusSyncTargets(ctx context.Context) ([]*model.BrowserEnvIndex, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("sqlite 未初始化")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT
		env_id,
		user_id,
		rpa_type,
		name,
		env_sequence,
		cdp_port,
		vnc_port,
		env_path,
		status,
		container_id,
		container_name,
		container_status,
		monitor_status,
		last_error,
		fingerprint_restored,
		has_browser_data,
		created_at,
		updated_at,
		deleted_at,
		last_started_at,
		last_stopped_at,
		last_checked_at
		FROM browser_envs
		WHERE status != ? AND status != ?
		ORDER BY env_sequence ASC`,
		model.BrowserEnvStatusDeleted,
		model.BrowserEnvStatusArchived,
	)
	if err != nil {
		return nil, fmt.Errorf("query browser_envs status sync targets failed: %w", err)
	}
	defer rows.Close()

	items := make([]*model.BrowserEnvIndex, 0)
	for rows.Next() {
		item, scanErr := scanBrowserEnvIndex(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate browser_envs status sync targets failed: %w", err)
	}
	return items, nil
}

// CountBrowserEnvIndexes 统计当前查询条件下的环境包数量。
//
// 它和列表共用 where 构建逻辑，保证 total 与 items 的过滤规则一致。
func (r *Repository) CountBrowserEnvIndexes(ctx context.Context, query model.ListBrowserEnvQuery) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("sqlite 未初始化")
	}
	whereSQL, args := buildBrowserEnvWhere(query)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM browser_envs `+whereSQL, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count browser_envs failed: %w", err)
	}
	return total, nil
}

// CountBrowserEnvByStatus 按生命周期状态统计环境包数量。
//
// 统计口径沿用当前查询条件；未显式传 status 时仍排除 deleted，避免默认列表和统计数字互相打架。
func (r *Repository) CountBrowserEnvByStatus(ctx context.Context, query model.ListBrowserEnvQuery) (map[string]int64, error) {
	return r.countBrowserEnvGroup(ctx, query, "status")
}

// CountBrowserEnvByRPAType 按 RPA 类型统计环境包数量。
//
// 这个统计用于前端做平台筛选摘要，不读取任何环境包文件内容。
func (r *Repository) CountBrowserEnvByRPAType(ctx context.Context, query model.ListBrowserEnvQuery) (map[string]int64, error) {
	return r.countBrowserEnvGroup(ctx, query, "rpa_type")
}

// countBrowserEnvGroup 执行受控字段的 GROUP BY 统计。
//
// groupField 不是外部输入，只能由上面的固定方法传入 status/rpa_type；
// 这样既保留通用统计逻辑，也避免把任意字符串拼进 SQL。
func (r *Repository) countBrowserEnvGroup(ctx context.Context, query model.ListBrowserEnvQuery, groupField string) (map[string]int64, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("sqlite 未初始化")
	}
	if groupField != "status" && groupField != "rpa_type" {
		return nil, fmt.Errorf("unsupported group field")
	}
	whereSQL, args := buildBrowserEnvWhere(query)
	rows, err := r.db.QueryContext(ctx, `SELECT `+groupField+`, COUNT(1) FROM browser_envs `+whereSQL+` GROUP BY `+groupField, args...)
	if err != nil {
		return nil, fmt.Errorf("count browser_envs by %s failed: %w", groupField, err)
	}
	defer rows.Close()

	result := map[string]int64{}
	for rows.Next() {
		var key string
		var count int64
		if err = rows.Scan(&key, &count); err != nil {
			return nil, fmt.Errorf("scan browser_envs group failed: %w", err)
		}
		result[key] = count
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate browser_envs group failed: %w", err)
	}
	return result, nil
}

// buildBrowserEnvWhere 统一生成列表和统计的过滤条件。
//
// 默认 status 为空时排除 deleted，这是用户确认的“假删除”展示规则；
// 如果显式传 status=deleted，则进入回收站视图，不再额外排除。
func buildBrowserEnvWhere(query model.ListBrowserEnvQuery) (string, []any) {
	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if query.UserID != "" {
		conditions = append(conditions, "user_id = ?")
		args = append(args, query.UserID)
	}
	if query.RPAType != "" {
		conditions = append(conditions, "rpa_type = ?")
		args = append(args, query.RPAType)
	}
	if query.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, query.Status)
	} else {
		conditions = append(conditions, "status != ?")
		args = append(args, model.BrowserEnvStatusDeleted)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

// scanBrowserEnvIndex 把数据库行转换成 BrowserEnvIndex。
//
// nullable 字段和 SQLite 0/1 布尔值只在 Repository 层处理，避免上层业务到处关心 SQL 扫描细节。
func scanBrowserEnvIndex(rows *sql.Rows) (*model.BrowserEnvIndex, error) {
	var item model.BrowserEnvIndex
	var containerID sql.NullString
	var containerName sql.NullString
	var lastError sql.NullString
	var fingerprintRestored int
	var hasBrowserData int
	var deletedAt sql.NullInt64
	var lastStartedAt sql.NullInt64
	var lastStoppedAt sql.NullInt64
	var lastCheckedAt sql.NullInt64

	if err := rows.Scan(
		&item.EnvID,
		&item.UserID,
		&item.RPAType,
		&item.Name,
		&item.EnvSequence,
		&item.CDPPort,
		&item.VNCPort,
		&item.EnvPath,
		&item.Status,
		&containerID,
		&containerName,
		&item.ContainerStatus,
		&item.MonitorStatus,
		&lastError,
		&fingerprintRestored,
		&hasBrowserData,
		&item.CreatedAt,
		&item.UpdatedAt,
		&deletedAt,
		&lastStartedAt,
		&lastStoppedAt,
		&lastCheckedAt,
	); err != nil {
		return nil, fmt.Errorf("scan browser_envs failed: %w", err)
	}
	item.ContainerID = nullableStringPtr(containerID)
	item.ContainerName = nullableStringPtr(containerName)
	item.LastError = nullableStringPtr(lastError)
	item.FingerprintRestored = fingerprintRestored == 1
	item.HasBrowserData = hasBrowserData == 1
	item.DeletedAt = nullableInt64Ptr(deletedAt)
	item.LastStartedAt = nullableInt64Ptr(lastStartedAt)
	item.LastStoppedAt = nullableInt64Ptr(lastStoppedAt)
	item.LastCheckedAt = nullableInt64Ptr(lastCheckedAt)
	return &item, nil
}

// boolToSQLiteInt 把 Go bool 转成 SQLite 中稳定的 0/1。
//
// SQLite 没有真正的 BOOLEAN 类型，统一在 Repository 层处理，避免 Dao 或 Service 到处散写转换。
func boolToSQLiteInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// nullableStringPtr 把 sql.NullString 归一成 JSON 友好的指针。
//
// 空值保持 nil，避免响应里把“不存在”和“空字符串”混成一种状态。
func nullableStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

// nullableInt64Ptr 把 sql.NullInt64 归一成 JSON 友好的指针。
//
// 时间字段为空时表示该生命周期动作还没有发生，不应返回 0 误导前端。
func nullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
