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
// 删除、状态刷新也应各自按业务动作命名，不要把所有方法塞进一个含义模糊的 Dao。
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

// ConfigModelHandler 是环境包配置修改动作的 Dao 入口。
//
// proxy/runtime/environment 这类修改最终会写文件，但 SQLite 索引仍需要同步状态和更新时间；
// 单独建 Handler 是为了避免把配置写入动作继续塞进 RuntimeModelHandler，保持调用语义清楚。
type ConfigModelHandler struct {
	repo *repository.Repository
}

// DeleteModelHandler 是环境包彻底删除动作的 Dao 入口。
//
// 删除动作会由 Service 先校验运行态和文件路径，再删除环境包目录；
// Dao 只负责最终移除 browser_envs 索引记录，不触碰文件系统。
type DeleteModelHandler struct {
	repo *repository.Repository
}

// StatusSyncModelHandler 是后台状态同步任务的 Dao 入口。
//
// 设计来源：
// - 用户要求框架里有定时任务自动刷新容器状态；
// - 同步任务既需要读待扫描环境包，也需要写运行态摘要；
// - 单独建 Handler 可以让调用处明确这是“后台状态同步”，不和 HTTP 列表查询或用户主动 run/stop 混在一起。
type StatusSyncModelHandler struct {
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

// NewConfigModelHandler 创建环境包配置修改 Dao。
func NewConfigModelHandler() *ConfigModelHandler {
	return &ConfigModelHandler{
		repo: repository.NewRepository(),
	}
}

// NewDeleteModelHandler 创建环境包删除 Dao。
func NewDeleteModelHandler() *DeleteModelHandler {
	return &DeleteModelHandler{
		repo: repository.NewRepository(),
	}
}

// NewStatusSyncModelHandler 创建后台状态同步 Dao。
func NewStatusSyncModelHandler() *StatusSyncModelHandler {
	return &StatusSyncModelHandler{
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

// UpdateBrowserEnvBackupState 更新备份/恢复后的环境资产状态。
//
// 备份会删除运行目录但保留 SQLite 资产索引；这个 Dao 方法把 Service 已确认的
// backupPath/checksum/status 等字段交给 Repository，不直接处理文件或 Docker。
func (h *RuntimeModelHandler) UpdateBrowserEnvBackupState(ctx context.Context, update *model.BrowserEnvBackupStateUpdate) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.UpdateBrowserEnvBackupState(ctx, update)
}

// UpdateBrowserEnvConfig 同步配置修改后的 browser_envs 索引状态。
//
// Dao 只保留“配置更新”这个业务动作名，不直接拼 SQL；
// 具体字段更新和 RowsAffected 判断交给 Repository。
func (h *ConfigModelHandler) UpdateBrowserEnvConfig(ctx context.Context, update *model.BrowserEnvConfigUpdate) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.UpdateBrowserEnvConfig(ctx, update)
}

// GetBrowserEnvIndexByID 按 envId 获取环境包索引。
//
// delete 必须先查索引状态，确认环境包存在且没有运行中容器，再进入物理删除流程。
func (h *DeleteModelHandler) GetBrowserEnvIndexByID(ctx context.Context, envID string) (*model.BrowserEnvIndex, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.GetBrowserEnvIndexByID(ctx, envID)
}

// DeleteBrowserEnvIndex 删除环境包索引记录。
//
// Dao 只表达“删除索引”这个业务动作名；具体 SQL 和 RowsAffected 判断交给 Repository。
func (h *DeleteModelHandler) DeleteBrowserEnvIndex(ctx context.Context, envID string) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.DeleteBrowserEnvIndex(ctx, envID)
}

// ListStatusSyncTargets 查询后台状态同步任务需要扫描的环境包。
//
// Dao 只保留业务动作名；不判断 Docker 状态、不读取文件，具体扫描逻辑由 Service 层负责。
func (h *StatusSyncModelHandler) ListStatusSyncTargets(ctx context.Context) ([]*model.BrowserEnvIndex, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("browser env dao 未初始化")
	}
	return h.repo.ListBrowserEnvStatusSyncTargets(ctx)
}

// UpdateBrowserEnvRuntime 更新后台同步得到的运行态摘要。
//
// 这里复用 Repository 的运行态更新能力，保证 run/stop/status sync 对 browser_envs 的写入字段一致。
func (h *StatusSyncModelHandler) UpdateBrowserEnvRuntime(ctx context.Context, update *model.BrowserEnvRuntimeUpdate) error {
	if h == nil || h.repo == nil {
		return errors.New("browser env dao 未初始化")
	}
	return h.repo.UpdateBrowserEnvRuntime(ctx, update)
}
