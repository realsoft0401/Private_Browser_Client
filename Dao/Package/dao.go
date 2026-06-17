package Package

import (
	"errors"

	model "private_browser_client/Models/Package"
	repository "private_browser_client/Repository/Package"
)

// CreateModelHandler 是 package 当前运行视图创建动作的 Dao 入口。
type CreateModelHandler struct {
	repo *repository.Repository
}

// RuntimeModelHandler 是 package 当前运行视图动作的 Dao 入口。
//
// 注意这里不是完整 package 资产 Dao，只服务 Client 本机当前运行视图。
type RuntimeModelHandler struct {
	repo *repository.Repository
}

// DeleteModelHandler 是 package 当前运行视图删除动作的 Dao 入口。
//
// 删除动作当前只服务本机运行摘要清理，不承担长期资产删除职责。
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

func (h *CreateModelHandler) CreateRuntimeView(view *model.RuntimeView) error {
	if h == nil || h.repo == nil {
		return errors.New("package dao 未初始化")
	}
	return h.repo.Create(view)
}

func (h *RuntimeModelHandler) GetByPackageID(packageID string) (*model.RuntimeView, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("package dao 未初始化")
	}
	return h.repo.GetByPackageID(packageID)
}

func (h *RuntimeModelHandler) UpdateRuntimeView(view *model.RuntimeView) error {
	if h == nil || h.repo == nil {
		return errors.New("package dao 未初始化")
	}
	return h.repo.Update(view)
}

func (h *DeleteModelHandler) DeleteRuntimeViewByPackageID(packageID string) error {
	if h == nil || h.repo == nil {
		return errors.New("package dao 未初始化")
	}
	return h.repo.DeleteByPackageID(packageID)
}
