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
