package BrowserEnv

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	model "private_browser_client/Models/BrowserEnv"
	common "private_browser_client/Repository/Common"
)

// Repository 封装 browser_envs 表的最小数据库访问。
//
// 当前先只接创建和按 envId 读取，因为这一步的目标是先把正式资产索引立住；
// 后续 backup/restore/revalidate 再继续在这一层补充更新和删除能力。
type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) Create(index *model.BrowserEnvIndex) error {
	if index == nil || strings.TrimSpace(index.EnvID) == "" {
		return common.ErrConflict
	}
	_, err := common.DB().Exec(
		`INSERT INTO browser_envs (
			env_id, user_id, rpa_type, name, env_sequence, cdp_port, vnc_port, env_path,
			status, container_id, container_name, container_status, monitor_status, last_error,
			backup_path, backup_checksum, backup_size, backup_at,
			fingerprint_restored, has_browser_data, created_at, updated_at, deleted_at,
			last_started_at, last_stopped_at, last_checked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		index.EnvID,
		index.UserID,
		index.RPAType,
		index.Name,
		index.EnvSequence,
		index.CDPPort,
		index.VNCPort,
		index.EnvPath,
		index.Status,
		index.ContainerID,
		index.ContainerName,
		index.ContainerStatus,
		index.MonitorStatus,
		index.LastError,
		index.BackupPath,
		index.BackupChecksum,
		index.BackupSize,
		index.BackupAt,
		boolToSQLiteInt(index.FingerprintRestored),
		boolToSQLiteInt(index.HasBrowserData),
		index.CreatedAt,
		index.UpdatedAt,
		index.DeletedAt,
		index.LastStartedAt,
		index.LastStoppedAt,
		index.LastCheckedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return common.ErrDuplicate
		}
		return fmt.Errorf("create browser env index failed: %w", err)
	}
	return nil
}

func (r *Repository) GetByEnvID(envID string) (*model.BrowserEnvIndex, error) {
	row := common.DB().QueryRow(
		`SELECT
			env_id, user_id, rpa_type, name, env_sequence, cdp_port, vnc_port, env_path,
			status, container_id, container_name, container_status, monitor_status, last_error,
			backup_path, backup_checksum, backup_size, backup_at,
			fingerprint_restored, has_browser_data, created_at, updated_at, deleted_at,
			last_started_at, last_stopped_at, last_checked_at
		FROM browser_envs WHERE env_id = ?`,
		envID,
	)
	return scanBrowserEnvIndex(row)
}

// ListBrowserEnvIndexes 查询当前过滤条件下的环境包摘要列表。
//
// 这层只做 SQLite 查询和扫描，不读取环境包目录、不碰 Docker；
// 列表、统计和详情的组合逻辑继续留在 Service 层，避免 Repository 反向承担业务聚合职责。
func (r *Repository) ListBrowserEnvIndexes(query model.ListBrowserEnvQuery) ([]*model.BrowserEnvIndex, error) {
	whereSQL, args := buildBrowserEnvWhere(query)
	offset := (query.Page - 1) * query.PageSize
	args = append(args, query.PageSize, offset)

	rows, err := common.DB().Query(
		`SELECT
			env_id, user_id, rpa_type, name, env_sequence, cdp_port, vnc_port, env_path,
			status, container_id, container_name, container_status, monitor_status, last_error,
			backup_path, backup_checksum, backup_size, backup_at,
			fingerprint_restored, has_browser_data, created_at, updated_at, deleted_at,
			last_started_at, last_stopped_at, last_checked_at
		FROM browser_envs `+whereSQL+`
		ORDER BY env_sequence ASC
		LIMIT ? OFFSET ?`,
		args...,
	)
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

// CountBrowserEnvIndexes 统计当前过滤条件下的环境包总数。
//
// 它和列表共用 where 构造逻辑，保证分页 total 和 items 使用完全一致的过滤口径。
func (r *Repository) CountBrowserEnvIndexes(query model.ListBrowserEnvQuery) (int64, error) {
	whereSQL, args := buildBrowserEnvWhere(query)
	var total int64
	if err := common.DB().QueryRow(`SELECT COUNT(1) FROM browser_envs `+whereSQL, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count browser_envs failed: %w", err)
	}
	return total, nil
}

// CountBrowserEnvByStatus 按生命周期状态统计环境包数量。
func (r *Repository) CountBrowserEnvByStatus(query model.ListBrowserEnvQuery) (map[string]int64, error) {
	return r.countBrowserEnvGroup(query, "status")
}

// CountBrowserEnvByRPAType 按 RPA 类型统计环境包数量。
func (r *Repository) CountBrowserEnvByRPAType(query model.ListBrowserEnvQuery) (map[string]int64, error) {
	return r.countBrowserEnvGroup(query, "rpa_type")
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBrowserEnvIndex(source scanner) (*model.BrowserEnvIndex, error) {
	var fingerprintRestored int
	var hasBrowserData int
	item := &model.BrowserEnvIndex{}
	err := source.Scan(
		&item.EnvID,
		&item.UserID,
		&item.RPAType,
		&item.Name,
		&item.EnvSequence,
		&item.CDPPort,
		&item.VNCPort,
		&item.EnvPath,
		&item.Status,
		&item.ContainerID,
		&item.ContainerName,
		&item.ContainerStatus,
		&item.MonitorStatus,
		&item.LastError,
		&item.BackupPath,
		&item.BackupChecksum,
		&item.BackupSize,
		&item.BackupAt,
		&fingerprintRestored,
		&hasBrowserData,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.DeletedAt,
		&item.LastStartedAt,
		&item.LastStoppedAt,
		&item.LastCheckedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, common.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan browser env index failed: %w", err)
	}
	item.FingerprintRestored = fingerprintRestored == 1
	item.HasBrowserData = hasBrowserData == 1
	return item, nil
}

func boolToSQLiteInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (r *Repository) countBrowserEnvGroup(query model.ListBrowserEnvQuery, groupField string) (map[string]int64, error) {
	if groupField != "status" && groupField != "rpa_type" {
		return nil, fmt.Errorf("unsupported group field")
	}
	whereSQL, args := buildBrowserEnvWhere(query)
	rows, err := common.DB().Query(`SELECT `+groupField+`, COUNT(1) FROM browser_envs `+whereSQL+` GROUP BY `+groupField, args...)
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

// UpdateConfig 同步配置修改后的轻量索引字段。
func (r *Repository) UpdateConfig(update *model.BrowserEnvConfigUpdate) error {
	if update == nil || strings.TrimSpace(update.EnvID) == "" {
		return common.ErrConflict
	}
	result, err := common.DB().Exec(
		`UPDATE browser_envs SET status = ?, last_error = ?, updated_at = ? WHERE env_id = ?`,
		update.Status,
		update.LastError,
		update.UpdatedAt,
		update.EnvID,
	)
	if err != nil {
		return fmt.Errorf("update browser env config failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read browser env config update result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}

// UpdateRuntime 同步运行态摘要字段。
func (r *Repository) UpdateRuntime(update *model.BrowserEnvRuntimeUpdate) error {
	if update == nil || strings.TrimSpace(update.EnvID) == "" {
		return common.ErrConflict
	}
	result, err := common.DB().Exec(
		`UPDATE browser_envs SET
			status = ?,
			env_sequence = COALESCE(?, env_sequence),
			cdp_port = COALESCE(?, cdp_port),
			vnc_port = COALESCE(?, vnc_port),
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
		update.EnvSequence,
		update.CDPPort,
		update.VNCPort,
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
		return fmt.Errorf("update browser env runtime failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read browser env runtime update result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}

// UpdateBackupState 同步 backup/restore 后的资产状态。
func (r *Repository) UpdateBackupState(update *model.BrowserEnvBackupStateUpdate) error {
	if update == nil || strings.TrimSpace(update.EnvID) == "" {
		return common.ErrConflict
	}
	result, err := common.DB().Exec(
		`UPDATE browser_envs SET
			status = ?,
			env_sequence = COALESCE(?, env_sequence),
			cdp_port = COALESCE(?, cdp_port),
			vnc_port = COALESCE(?, vnc_port),
			container_id = ?,
			container_name = ?,
			container_status = ?,
			monitor_status = ?,
			last_error = ?,
			has_browser_data = ?,
			backup_path = ?,
			backup_checksum = ?,
			backup_size = ?,
			backup_at = ?,
			updated_at = ?,
			last_started_at = ?,
			last_stopped_at = ?,
			last_checked_at = ?
		WHERE env_id = ?`,
		update.Status,
		update.EnvSequence,
		update.CDPPort,
		update.VNCPort,
		update.ContainerID,
		update.ContainerName,
		update.ContainerStatus,
		update.MonitorStatus,
		update.LastError,
		boolToSQLiteInt(update.HasBrowserData),
		update.BackupPath,
		update.BackupChecksum,
		update.BackupSize,
		update.BackupAt,
		update.UpdatedAt,
		update.LastStartedAt,
		update.LastStoppedAt,
		update.LastCheckedAt,
		update.EnvID,
	)
	if err != nil {
		return fmt.Errorf("update browser env backup state failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read browser env backup state update result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteByEnvID(envID string) error {
	result, err := common.DB().Exec(`DELETE FROM browser_envs WHERE env_id = ?`, strings.TrimSpace(envID))
	if err != nil {
		return fmt.Errorf("delete browser env index failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read browser env delete result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}
