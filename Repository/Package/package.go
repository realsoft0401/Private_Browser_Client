package Package

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	model "private_browser_client/Models/Package"
	common "private_browser_client/Repository/Common"
)

// Repository 是 package 当前运行视图的持久化入口。
//
// 这里仍然不是完整 package 资产仓库，只是 Client 本机当前运行视图仓库。
// 真正的 package 长期资产内容后续仍然在环境包目录及相关资产文件中。
type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) Create(view *model.RuntimeView) error {
	if view == nil || view.PackageID == "" {
		return common.ErrConflict
	}
	_, err := common.DB().Exec(
		`INSERT INTO package_runtime_views (
			package_id, current_run_id, current_slot_id, runtime_status, last_run_at, last_stop_at, last_error
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		view.PackageID, view.CurrentRunID, view.CurrentSlotID, view.RuntimeStatus, view.LastRunAt, view.LastStopAt, view.LastError,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return common.ErrDuplicate
		}
		return fmt.Errorf("create package runtime view failed: %w", err)
	}
	return nil
}

func (r *Repository) GetByPackageID(packageID string) (*model.RuntimeView, error) {
	row := common.DB().QueryRow(
		`SELECT package_id, current_run_id, current_slot_id, runtime_status, last_run_at, last_stop_at, last_error
		   FROM package_runtime_views WHERE package_id = ?`,
		packageID,
	)
	view, err := scanRuntimeView(row)
	if err != nil {
		return nil, err
	}
	return view, nil
}

func (r *Repository) Update(view *model.RuntimeView) error {
	if view == nil || view.PackageID == "" {
		return common.ErrConflict
	}
	result, err := common.DB().Exec(
		`UPDATE package_runtime_views SET
			current_run_id = ?, current_slot_id = ?, runtime_status = ?, last_run_at = ?, last_stop_at = ?, last_error = ?
		  WHERE package_id = ?`,
		view.CurrentRunID, view.CurrentSlotID, view.RuntimeStatus, view.LastRunAt, view.LastStopAt, view.LastError, view.PackageID,
	)
	if err != nil {
		return fmt.Errorf("update package runtime view failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read package runtime view update result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteByPackageID(packageID string) error {
	result, err := common.DB().Exec(`DELETE FROM package_runtime_views WHERE package_id = ?`, packageID)
	if err != nil {
		return fmt.Errorf("delete package runtime view failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read package runtime view delete result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRuntimeView(source scanner) (*model.RuntimeView, error) {
	view := &model.RuntimeView{}
	err := source.Scan(
		&view.PackageID, &view.CurrentRunID, &view.CurrentSlotID, &view.RuntimeStatus, &view.LastRunAt, &view.LastStopAt, &view.LastError,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, common.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan package runtime view failed: %w", err)
	}
	return view, nil
}
