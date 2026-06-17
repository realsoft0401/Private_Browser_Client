package Slot

import (
	"errors"

	model "private_browser_client/Models/Slot"
	repository "private_browser_client/Repository/Slot"
)

// CreateModelHandler 是 slot 创建动作的 Dao 入口。
//
// 这里继续沿用 old 的阅读方式：Dao 表达“业务动作名”，Repository 负责实际持久化。
// 这样后续 Service 调用时，仍然是一眼能看懂的动作语义，而不是直接堆 Repository 方法。
type CreateModelHandler struct {
	repo *repository.Repository
}

// RuntimeModelHandler 是 slot 当前态动作的 Dao 入口。
//
// slot 的 loading/occupied/releasing/waiting 更新都先收在这里，
// 后续如果要加更多 slot 动作，也继续沿用这条语义分组，不要把所有动作塞进一个模糊 Handler。
type RuntimeModelHandler struct {
	repo *repository.Repository
}

// DeleteModelHandler 是 slot 删除动作的 Dao 入口。
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

func (h *CreateModelHandler) CreateSlot(slot *model.Slot) error {
	if h == nil || h.repo == nil {
		return errors.New("slot dao 未初始化")
	}
	return h.repo.Create(slot)
}

func (h *RuntimeModelHandler) GetSlotByID(slotID string) (*model.Slot, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("slot dao 未初始化")
	}
	return h.repo.GetBySlotID(slotID)
}

func (h *RuntimeModelHandler) ListSlots() ([]*model.Slot, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("slot dao 未初始化")
	}
	return h.repo.List()
}

func (h *RuntimeModelHandler) UpdateSlot(slot *model.Slot) error {
	if h == nil || h.repo == nil {
		return errors.New("slot dao 未初始化")
	}
	return h.repo.Update(slot)
}

func (h *DeleteModelHandler) DeleteSlotByID(slotID string) error {
	if h == nil || h.repo == nil {
		return errors.New("slot dao 未初始化")
	}
	return h.repo.DeleteBySlotID(slotID)
}
