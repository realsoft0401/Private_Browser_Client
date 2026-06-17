package Runtime

import (
	"errors"
	"strings"
	"time"

	runtimeDao "private_browser_client/Dao/Runtime"
	model "private_browser_client/Models/Runtime"
)

type Service struct{}

// NewService 创建运行关系业务服务。
//
// 这层是新模型最关键的地方之一，因为 package 和 slot 允许松耦合，
// 所以必须保留独立的 runtime relation Service，而不能把所有状态直接塞回 package 或 slot。
func NewService() *Service {
	return &Service{}
}

// CreateRelation 创建当前运行关系。
func (s *Service) CreateRelation(runID string, packageID string, slotID string) (*model.RuntimeRelation, error) {
	runID = strings.TrimSpace(runID)
	packageID = strings.TrimSpace(packageID)
	slotID = strings.TrimSpace(slotID)
	if runID == "" || packageID == "" || slotID == "" {
		return nil, errors.New("runId、packageId、slotId 不能为空")
	}

	now := time.Now().Unix()
	relation := &model.RuntimeRelation{
		RunID:     runID,
		PackageID: packageID,
		SlotID:    slotID,
		Status:    model.StatusLoading,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := runtimeDao.NewCreateModelHandler().CreateRuntimeRelation(relation); err != nil {
		return nil, err
	}
	return relation, nil
}

// GetByRunID 按 runId 查询当前运行关系。
func (s *Service) GetByRunID(runID string) (*model.RuntimeRelation, error) {
	return runtimeDao.NewRuntimeModelHandler().GetByRunID(strings.TrimSpace(runID))
}

// GetByPackageID 按 packageId 查询当前运行关系。
func (s *Service) GetByPackageID(packageID string) (*model.RuntimeRelation, error) {
	return runtimeDao.NewRuntimeModelHandler().GetByPackageID(strings.TrimSpace(packageID))
}

// GetBySlotID 按 slotId 查询当前运行关系。
func (s *Service) GetBySlotID(slotID string) (*model.RuntimeRelation, error) {
	return runtimeDao.NewRuntimeModelHandler().GetBySlotID(strings.TrimSpace(slotID))
}

// UpdateRelation 更新当前运行关系。
func (s *Service) UpdateRelation(relation *model.RuntimeRelation) error {
	if relation != nil {
		relation.UpdatedAt = time.Now().Unix()
	}
	return runtimeDao.NewRuntimeModelHandler().UpdateRuntimeRelation(relation)
}

// DeleteRelationByRunID 删除当前运行关系。
func (s *Service) DeleteRelationByRunID(runID string) error {
	return runtimeDao.NewDeleteModelHandler().DeleteRuntimeRelationByRunID(strings.TrimSpace(runID))
}
