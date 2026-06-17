package Runtime

import (
	"errors"

	model "private_browser_client/Models/Runtime"
	repository "private_browser_client/Repository/Runtime"
)

// CreateModelHandler 是运行关系创建动作的 Dao 入口。
type CreateModelHandler struct {
	repo *repository.Repository
}

// RuntimeModelHandler 是运行关系当前态动作的 Dao 入口。
//
// 当前阶段先只处理“当前关系”，不承接长历史归档；
// 长历史后续仍归 Node Server。
type RuntimeModelHandler struct {
	repo *repository.Repository
}

// DeleteModelHandler 是运行关系结束删除动作的 Dao 入口。
type DeleteModelHandler struct {
	repo *repository.Repository
}

func NewCreateModelHandler() *CreateModelHandler {
	return &CreateModelHandler{
		repo: repository.NewRepository(),
	}
}

func NewRuntimeModelHandler() *RuntimeModelHandler {
	return &RuntimeModelHandler{
		repo: repository.NewRepository(),
	}
}

func NewDeleteModelHandler() *DeleteModelHandler {
	return &DeleteModelHandler{
		repo: repository.NewRepository(),
	}
}

func (h *CreateModelHandler) CreateRuntimeRelation(relation *model.RuntimeRelation) error {
	if h == nil || h.repo == nil {
		return errors.New("runtime dao 未初始化")
	}
	return h.repo.Create(relation)
}

func (h *RuntimeModelHandler) GetByRunID(runID string) (*model.RuntimeRelation, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("runtime dao 未初始化")
	}
	return h.repo.GetByRunID(runID)
}

func (h *RuntimeModelHandler) GetByPackageID(packageID string) (*model.RuntimeRelation, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("runtime dao 未初始化")
	}
	return h.repo.GetByPackageID(packageID)
}

func (h *RuntimeModelHandler) GetBySlotID(slotID string) (*model.RuntimeRelation, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("runtime dao 未初始化")
	}
	return h.repo.GetBySlotID(slotID)
}

func (h *RuntimeModelHandler) UpdateRuntimeRelation(relation *model.RuntimeRelation) error {
	if h == nil || h.repo == nil {
		return errors.New("runtime dao 未初始化")
	}
	return h.repo.Update(relation)
}

func (h *DeleteModelHandler) DeleteRuntimeRelationByRunID(runID string) error {
	if h == nil || h.repo == nil {
		return errors.New("runtime dao 未初始化")
	}
	return h.repo.DeleteByRunID(runID)
}
