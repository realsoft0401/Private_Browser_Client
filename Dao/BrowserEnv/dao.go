package BrowserEnv

import (
	"context"
	"errors"

	model "private_browser_client/Models/BrowserEnv"
	repository "private_browser_client/Repository/BrowserEnv"
)

var ErrDuplicateBrowserEnv = repository.ErrDuplicateBrowserEnv
var ErrBrowserEnvNotFound = repository.ErrBrowserEnvNotFound

// CreateModelHandler 是创建环境包索引的 Dao 业务动作入口。
//
// 设计来源：
// - 用户参考 pure-forufamily-admin-backend 后确认，希望保留 “ModelHandler + 业务动作方法” 的阅读方式；
// - 同时不能让 Dao 继续膨胀成直接堆 SQL 的地方，所以 Dao 只整理业务动作并转交 Repository；
// - 这样 Service 读起来仍然像调用业务动作，底层数据库细节则集中在 Repository。
type CreateModelHandler struct {
	repo *repository.Repository
}

// ListModelHandler 是查询环境包索引列表的 Dao 业务动作入口。
//
// 它和 CreateModelHandler 分开，是为了让“创建”和“查询”两个动作在调用处一眼可见；
// 后续软删除、状态刷新也应各自按业务动作命名，不要把所有方法塞进一个含义模糊的 Dao。
type ListModelHandler struct {
	repo *repository.Repository
}

// RuntimeModelHandler 是环境包运行态动作的 Dao 入口。
//
// run、stop、状态刷新都会围绕 browser_envs 的运行摘要更新；
// 单独建这个 Handler 是为了避免创建/列表 Dao 继续膨胀，也让调用处能看出当前是“运行态动作”。
type RuntimeModelHandler struct {
	repo *repository.Repository
}

// NewCreateModelHandler 创建环境包索引 Dao。
//
// 当前不在这里做事务编排；如果后续创建环境包需要同时写多张索引表，
// 再把事务边界提升到 Service 或专门的 UnitOfWork，不要让 Repository 自己悄悄开事务。
func NewCreateModelHandler() *CreateModelHandler {
	return &CreateModelHandler{
		repo: repository.NewRepository(),
	}
}

// NewListModelHandler 创建环境包列表 Dao。
//
// 当前列表接口只读 browser_envs 索引表，不反向扫描文件夹；
// 如果后续要做目录存在性校验，应作为 Service 层的额外校验策略，而不是塞进 Repository 查询。
func NewListModelHandler() *ListModelHandler {
	return &ListModelHandler{
		repo: repository.NewRepository(),
	}
}

// NewRuntimeModelHandler 创建环境包运行态 Dao。
func NewRuntimeModelHandler() *RuntimeModelHandler {
	return &RuntimeModelHandler{
		repo: repository.NewRepository(),
	}
}

// CreateBrowserEnvIndex 保存 browser_envs 元数据索引。
//
// 职责边界：
// - 接收 Service 已经确认成功创建环境包后的索引模型；
// - 不读取或修改 profile 文件，不做端口分配，不做 Docker 状态探测；
// - 只把“创建环境包”这个业务动作映射成 Repository 写入。
func (h *CreateModelHandler) CreateBrowserEnvIndex(ctx context.Context, record *model.BrowserEnvIndex) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.InsertBrowserEnvIndex(ctx, record)
}

// ListBrowserEnvIndexes 查询环境包分页列表。
//
// 职责边界：
// - Dao 保留业务动作名称，方便按用户的旧框架习惯阅读；
// - 不直接拼 SQL，不扫描环境包目录，不展开 profile 内容；
// - 具体 SQL 过滤、分页、统计交给 Repository。
func (h *ListModelHandler) ListBrowserEnvIndexes(ctx context.Context, query model.ListBrowserEnvQuery) ([]*model.BrowserEnvIndex, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.ListBrowserEnvIndexes(ctx, query)
}

// CountBrowserEnvIndexes 统计当前查询条件下的环境包总数。
func (h *ListModelHandler) CountBrowserEnvIndexes(ctx context.Context, query model.ListBrowserEnvQuery) (int64, error) {
	if h == nil || h.repo == nil {
		return 0, errors.New("browser env dao 未初始化")
	}
	return h.repo.CountBrowserEnvIndexes(ctx, query)
}

// CountBrowserEnvByStatus 按环境包生命周期状态统计。
func (h *ListModelHandler) CountBrowserEnvByStatus(ctx context.Context, query model.ListBrowserEnvQuery) (map[string]int64, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.CountBrowserEnvByStatus(ctx, query)
}

// CountBrowserEnvByRPAType 按 RPA 类型统计。
func (h *ListModelHandler) CountBrowserEnvByRPAType(ctx context.Context, query model.ListBrowserEnvQuery) (map[string]int64, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.CountBrowserEnvByRPAType(ctx, query)
}

// GetBrowserEnvIndexByID 按 envId 获取环境包索引。
//
// run 必须从数据库确认 envPath/status，不允许绕过索引直接猜目录。
func (h *RuntimeModelHandler) GetBrowserEnvIndexByID(ctx context.Context, envID string) (*model.BrowserEnvIndex, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.GetBrowserEnvIndexByID(ctx, envID)
}

// UpdateBrowserEnvRuntime 更新环境包运行态摘要。
//
// 这里只转交 Repository，不写 SQL；中文业务错误仍由 Service 层决定。
func (h *RuntimeModelHandler) UpdateBrowserEnvRuntime(ctx context.Context, update *model.BrowserEnvRuntimeUpdate) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.UpdateBrowserEnvRuntime(ctx, update)
}
