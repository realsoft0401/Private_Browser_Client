package Slot

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	model "private_browser_client/Models/Slot"
	common "private_browser_client/Repository/Common"
)

// Repository 是 slot 当前态的持久化入口。
//
// 这次正式接回 SQLite 后，slot 不再只是内存 map：
// - create-slot 后能跨进程重启保留；
// - run/stop/reinit 的当前态有稳定事实源；
// - 后续 Node/排障页面也不会因为服务重启把资源位全部“清空”。
//
// 职责边界保持不变：
// - 只负责 slot 当前态读写；
// - 不做分配决策；
// - 不直接碰 Docker 和平台接口。
type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) Create(slot *model.Slot) error {
	if slot == nil || slot.SlotID == "" {
		return common.ErrConflict
	}
	_, err := common.DB().Exec(
		`INSERT INTO slots (
			slot_id, status, current_package_id, current_run_id, container_id, container_name,
			runtime_image, container_status, cdp_port, vnc_port, last_error, initialized_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slot.SlotID, slot.Status, slot.CurrentPackageID, slot.CurrentRunID, slot.ContainerID, slot.ContainerName,
		slot.RuntimeImage, slot.ContainerStatus, slot.CDPPort, slot.VNCPort, slot.LastError, slot.InitializedAt, slot.UpdatedAt,
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return common.ErrDuplicate
		}
		return fmt.Errorf("create slot failed: %w", err)
	}
	return nil
}

func (r *Repository) GetBySlotID(slotID string) (*model.Slot, error) {
	row := common.DB().QueryRow(
		`SELECT slot_id, status, current_package_id, current_run_id, container_id, container_name,
		        runtime_image, container_status, cdp_port, vnc_port, last_error, initialized_at, updated_at
		   FROM slots WHERE slot_id = ?`,
		slotID,
	)
	slot, err := scanSlot(row)
	if err != nil {
		return nil, err
	}
	return slot, nil
}

func (r *Repository) List() ([]*model.Slot, error) {
	rows, err := common.DB().Query(
		`SELECT slot_id, status, current_package_id, current_run_id, container_id, container_name,
		        runtime_image, container_status, cdp_port, vnc_port, last_error, initialized_at, updated_at
		   FROM slots ORDER BY slot_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list slots failed: %w", err)
	}
	defer rows.Close()

	result := make([]*model.Slot, 0)
	for rows.Next() {
		slot, scanErr := scanSlot(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, slot)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate slots failed: %w", err)
	}
	return result, nil
}

func (r *Repository) Update(slot *model.Slot) error {
	if slot == nil || slot.SlotID == "" {
		return common.ErrConflict
	}
	result, err := common.DB().Exec(
		`UPDATE slots SET
			status = ?, current_package_id = ?, current_run_id = ?, container_id = ?, container_name = ?,
			runtime_image = ?, container_status = ?, cdp_port = ?, vnc_port = ?, last_error = ?, initialized_at = ?, updated_at = ?
		  WHERE slot_id = ?`,
		slot.Status, slot.CurrentPackageID, slot.CurrentRunID, slot.ContainerID, slot.ContainerName,
		slot.RuntimeImage, slot.ContainerStatus, slot.CDPPort, slot.VNCPort, slot.LastError, slot.InitializedAt, slot.UpdatedAt,
		slot.SlotID,
	)
	if err != nil {
		return fmt.Errorf("update slot failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read slot update result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteBySlotID(slotID string) error {
	result, err := common.DB().Exec(`DELETE FROM slots WHERE slot_id = ?`, slotID)
	if err != nil {
		return fmt.Errorf("delete slot failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read slot delete result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSlot(source scanner) (*model.Slot, error) {
	slot := &model.Slot{}
	err := source.Scan(
		&slot.SlotID, &slot.Status, &slot.CurrentPackageID, &slot.CurrentRunID, &slot.ContainerID, &slot.ContainerName,
		&slot.RuntimeImage, &slot.ContainerStatus, &slot.CDPPort, &slot.VNCPort, &slot.LastError, &slot.InitializedAt, &slot.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, common.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan slot failed: %w", err)
	}
	return slot, nil
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "constraint failed")
}
