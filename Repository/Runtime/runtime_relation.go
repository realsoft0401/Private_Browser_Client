package Runtime

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	model "private_browser_client/Models/Runtime"
	common "private_browser_client/Repository/Common"
)

// Repository 是当前运行关系的持久化入口。
//
// 新模型里 runtime relation 是独立对象，因此它也必须有独立表，
// 不能继续退回“只在 package 或 slot 上挂几个字段”的旧做法。
type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) Create(relation *model.RuntimeRelation) error {
	if relation == nil || relation.RunID == "" || relation.PackageID == "" || relation.SlotID == "" {
		return common.ErrConflict
	}
	_, err := common.DB().Exec(
		`INSERT INTO runtime_relations (
			run_id, package_id, slot_id, status, last_error, started_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		relation.RunID, relation.PackageID, relation.SlotID, relation.Status, relation.LastError, relation.StartedAt, relation.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			if strings.Contains(err.Error(), "run_id") {
				return common.ErrDuplicate
			}
			return common.ErrConflict
		}
		return fmt.Errorf("create runtime relation failed: %w", err)
	}
	return nil
}

func (r *Repository) GetByRunID(runID string) (*model.RuntimeRelation, error) {
	row := common.DB().QueryRow(
		`SELECT run_id, package_id, slot_id, status, last_error, started_at, updated_at
		   FROM runtime_relations WHERE run_id = ?`,
		runID,
	)
	return scanRelation(row)
}

func (r *Repository) GetByPackageID(packageID string) (*model.RuntimeRelation, error) {
	row := common.DB().QueryRow(
		`SELECT run_id, package_id, slot_id, status, last_error, started_at, updated_at
		   FROM runtime_relations WHERE package_id = ?`,
		packageID,
	)
	return scanRelation(row)
}

func (r *Repository) GetBySlotID(slotID string) (*model.RuntimeRelation, error) {
	row := common.DB().QueryRow(
		`SELECT run_id, package_id, slot_id, status, last_error, started_at, updated_at
		   FROM runtime_relations WHERE slot_id = ?`,
		slotID,
	)
	return scanRelation(row)
}

func (r *Repository) Update(relation *model.RuntimeRelation) error {
	if relation == nil || relation.RunID == "" || relation.PackageID == "" || relation.SlotID == "" {
		return common.ErrConflict
	}
	result, err := common.DB().Exec(
		`UPDATE runtime_relations SET
			package_id = ?, slot_id = ?, status = ?, last_error = ?, started_at = ?, updated_at = ?
		  WHERE run_id = ?`,
		relation.PackageID, relation.SlotID, relation.Status, relation.LastError, relation.StartedAt, relation.UpdatedAt, relation.RunID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return common.ErrConflict
		}
		return fmt.Errorf("update runtime relation failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read runtime relation update result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteByRunID(runID string) error {
	result, err := common.DB().Exec(`DELETE FROM runtime_relations WHERE run_id = ?`, runID)
	if err != nil {
		return fmt.Errorf("delete runtime relation failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read runtime relation delete result failed: %w", err)
	}
	if affected == 0 {
		return common.ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRelation(source scanner) (*model.RuntimeRelation, error) {
	relation := &model.RuntimeRelation{}
	err := source.Scan(
		&relation.RunID, &relation.PackageID, &relation.SlotID, &relation.Status, &relation.LastError, &relation.StartedAt, &relation.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, common.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan runtime relation failed: %w", err)
	}
	return relation, nil
}
