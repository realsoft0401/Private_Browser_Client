package BrowserEnv

import (
	"errors"

	model "private_browser_client/Models/BrowserEnv"
	repository "private_browser_client/Repository/BrowserEnv"
)

// CreateModelHandler 是 browser_envs 创建动作的 Dao 入口。
type CreateModelHandler struct {
	repo *repository.Repository
}

// RuntimeModelHandler 是 browser_envs 单条读取动作的 Dao 入口。
type RuntimeModelHandler struct {
	repo *repository.Repository
}

// ListModelHandler 是 browser_envs 列表与统计查询入口。
type ListModelHandler struct {
	repo *repository.Repository
}

// ConfigModelHandler 是 browser_envs 配置轻量回写入口。
type ConfigModelHandler struct {
	repo *repository.Repository
}

// DeleteModelHandler 是 browser_envs 索引删除入口。
type DeleteModelHandler struct {
	repo *repository.Repository
}

func NewCreateModelHandler() *CreateModelHandler {
	return &CreateModelHandler{repo: repository.NewRepository()}
}

func NewRuntimeModelHandler() *RuntimeModelHandler {
	return &RuntimeModelHandler{repo: repository.NewRepository()}
}

func NewListModelHandler() *ListModelHandler {
	return &ListModelHandler{repo: repository.NewRepository()}
}

func NewConfigModelHandler() *ConfigModelHandler {
	return &ConfigModelHandler{repo: repository.NewRepository()}
}

func NewDeleteModelHandler() *DeleteModelHandler {
	return &DeleteModelHandler{repo: repository.NewRepository()}
}

func (h *CreateModelHandler) CreateBrowserEnvIndex(index *model.BrowserEnvIndex) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.Create(index)
}

func (h *RuntimeModelHandler) GetBrowserEnvIndexByID(envID string) (*model.BrowserEnvIndex, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.GetByEnvID(envID)
}

func (h *RuntimeModelHandler) UpdateBrowserEnvRuntime(update *model.BrowserEnvRuntimeUpdate) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.UpdateRuntime(update)
}

func (h *RuntimeModelHandler) UpdateBrowserEnvBackupState(update *model.BrowserEnvBackupStateUpdate) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.UpdateBackupState(update)
}

func (h *ListModelHandler) ListBrowserEnvIndexes(query model.ListBrowserEnvQuery) ([]*model.BrowserEnvIndex, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.ListBrowserEnvIndexes(query)
}

func (h *ListModelHandler) CountBrowserEnvIndexes(query model.ListBrowserEnvQuery) (int64, error) {
	if h == nil || h.repo == nil {
		return 0, errors.New("browser env dao 未初始化")
	}
	return h.repo.CountBrowserEnvIndexes(query)
}

func (h *ListModelHandler) CountBrowserEnvByStatus(query model.ListBrowserEnvQuery) (map[string]int64, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.CountBrowserEnvByStatus(query)
}

func (h *ListModelHandler) CountBrowserEnvByRPAType(query model.ListBrowserEnvQuery) (map[string]int64, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.CountBrowserEnvByRPAType(query)
}

func (h *ConfigModelHandler) UpdateBrowserEnvConfig(update *model.BrowserEnvConfigUpdate) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.UpdateConfig(update)
}

func (h *DeleteModelHandler) DeleteBrowserEnvIndex(envID string) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.DeleteByEnvID(envID)
}
