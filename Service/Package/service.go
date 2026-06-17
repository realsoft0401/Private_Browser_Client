package Package

import (
	"errors"
	"fmt"
	"strings"
	"time"

	packageDao "private_browser_client/Dao/Package"
	model "private_browser_client/Models/Package"
	runtimeModel "private_browser_client/Models/Runtime"
	slotModel "private_browser_client/Models/Slot"
	common "private_browser_client/Repository/Common"
	platformService "private_browser_client/Service/Platform"
	runtimeService "private_browser_client/Service/Runtime"
	slotService "private_browser_client/Service/Slot"
)

type Service struct{}

// NewService 创建 package 当前运行视图业务服务。
//
// 注意这层不是完整 package 资产服务，而是 Client 本机运行视图服务。
// 真正的长期资产目录、登录态、proxy、fingerprint 后续仍会在更完整的 package 业务实现中接回来。
func NewService() *Service {
	return &Service{}
}

// CreateRuntimeView 创建 package 当前运行视图。
func (s *Service) CreateRuntimeView(packageID string) (*model.RuntimeView, error) {
	packageID = strings.TrimSpace(packageID)
	if packageID == "" {
		return nil, errors.New("packageId 不能为空")
	}

	view := &model.RuntimeView{
		PackageID:     packageID,
		RuntimeStatus: model.StatusCreated,
	}
	if err := packageDao.NewCreateModelHandler().CreateRuntimeView(view); err != nil {
		return nil, err
	}
	return view, nil
}

// GetByPackageID 查询 package 当前运行视图。
func (s *Service) GetByPackageID(packageID string) (*model.RuntimeView, error) {
	return packageDao.NewRuntimeModelHandler().GetByPackageID(strings.TrimSpace(packageID))
}

// UpdateRuntimeView 更新 package 当前运行视图。
func (s *Service) UpdateRuntimeView(view *model.RuntimeView) error {
	if view == nil {
		return nil
	}
	now := time.Now().Unix()
	if view.RuntimeStatus == model.StatusRunning && view.LastRunAt == nil {
		view.LastRunAt = &now
	}
	if view.RuntimeStatus == model.StatusStopped || view.RuntimeStatus == model.StatusCreated {
		view.LastStopAt = &now
	}
	return packageDao.NewRuntimeModelHandler().UpdateRuntimeView(view)
}

// EnsureRuntimeView 确保 package 在本机有一条当前运行视图记录。
//
// 新模型里“包是包、容器是容器”，但 run/stop 的用户主状态仍然看 package 侧。
// 因此即使底层 아직还没接 SQLite，这里也要保证 package 视图对象始终存在，后续才能稳定回写状态。
func (s *Service) EnsureRuntimeView(packageID string) (*model.RuntimeView, error) {
	view, err := s.GetByPackageID(packageID)
	if err == nil {
		return view, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return nil, err
	}
	return s.CreateRuntimeView(packageID)
}

// RunPackage 把指定 package 放进指定 slot 执行。
//
// 当前阶段这里先只完成“Client 本机状态机收口”：
// - 校验 slot 是否存在且空闲
// - 校验 package 当前没有活动运行关系
// - 建立 runtime relation
// - 回写 package 主状态和 slot 当前态
//
// 真正的配置包加载、容器启动、CDP 探测和 SSE 分阶段事件，后续仍会继续挂到这条主链路上。
//
// ******** 平台端接口接入点 7：
// 当后续平台端已经提供“run 前放行/任务登记/是否允许本次 package 进入指定 slot”的接口后，
// 必须在这里、任何本机运行关系写入之前调用。
// 这样可以避免平台未放行但 Client 已经先把 package/slot 改成 running。
func (s *Service) RunPackage(packageID string, slotID string) (*model.RuntimeView, error) {
	packageID = strings.TrimSpace(packageID)
	slotID = strings.TrimSpace(slotID)
	if packageID == "" || slotID == "" {
		return nil, errors.New("packageId 和 slotId 不能为空")
	}
	if err := platformService.GetPackageRuntimeSync().BeforeRunPackage(packageID, slotID); err != nil {
		return nil, err
	}

	slotSvc := slotService.NewService()
	runtimeSvc := runtimeService.NewService()

	slot, err := slotSvc.GetSlotByID(slotID)
	if err != nil {
		return nil, err
	}
	if slot.Status != slotModel.StatusWaiting {
		return nil, common.ErrConflict
	}
	if _, err = runtimeSvc.GetByPackageID(packageID); err == nil {
		return nil, common.ErrConflict
	} else if !errors.Is(err, common.ErrNotFound) {
		return nil, err
	}
	if _, err = runtimeSvc.GetBySlotID(slotID); err == nil {
		return nil, common.ErrConflict
	} else if !errors.Is(err, common.ErrNotFound) {
		return nil, err
	}

	view, err := s.EnsureRuntimeView(packageID)
	if err != nil {
		return nil, err
	}
	if view.RuntimeStatus != model.StatusCreated && view.RuntimeStatus != model.StatusStopped && view.RuntimeStatus != model.StatusError {
		return nil, common.ErrConflict
	}

	runID := buildRunID(packageID)
	relation, err := runtimeSvc.CreateRelation(runID, packageID, slotID)
	if err != nil {
		return nil, err
	}
	relation.Status = runtimeModel.StatusRunning
	if err = runtimeSvc.UpdateRelation(relation); err != nil {
		return nil, err
	}

	slot.Status = slotModel.StatusOccupied
	slot.CurrentPackageID = &packageID
	slot.CurrentRunID = &runID
	slot.LastError = nil
	if err = slotSvc.UpdateSlot(slot); err != nil {
		return nil, err
	}

	view.CurrentRunID = &runID
	view.CurrentSlotID = &slotID
	view.RuntimeStatus = model.StatusRunning
	view.LastError = nil
	if err = s.UpdateRuntimeView(view); err != nil {
		return nil, err
	}

	// ******** 平台端接口接入点 8：
	// 当平台端后续提供“run 成功后的状态回告、中心任务推进、slot 占用登记”等接口时，
	// 直接在这里调用。
	// 当前先不接，是因为这期只先完成 Client 本机 run 状态机，不引入平台编排依赖。
	result, err := s.GetByPackageID(packageID)
	if err != nil {
		return nil, err
	}
	if err = platformService.GetPackageRuntimeSync().AfterRunPackage(result); err != nil {
		return nil, err
	}
	return result, nil
}

// StopPackage 结束 package 当前运行关系并释放 slot。
//
// 这里遵守前面已经收口的口径：
// - 包状态是用户主状态
// - slot 状态和 package 状态允许存在短暂时间差
// - 当前骨架先在一个同步动作里收回 waiting/stopped，避免把未释放关系留成脏状态
//
// ******** 平台端接口接入点 9：
// 当后续平台端已经提供“stop 前任务确认/是否允许收口当前运行关系”的接口后，
// 需要在这里统一接入，而不是把 stop 放行逻辑拆到 HTTP 层或 Node 临时脚本。
func (s *Service) StopPackage(packageID string, slotID string) (*model.RuntimeView, error) {
	packageID = strings.TrimSpace(packageID)
	slotID = strings.TrimSpace(slotID)
	if packageID == "" || slotID == "" {
		return nil, errors.New("packageId 和 slotId 不能为空")
	}
	if err := platformService.GetPackageRuntimeSync().BeforeStopPackage(packageID, slotID); err != nil {
		return nil, err
	}

	slotSvc := slotService.NewService()
	runtimeSvc := runtimeService.NewService()

	relation, err := runtimeSvc.GetByPackageID(packageID)
	if err != nil {
		return nil, err
	}
	if relation.SlotID != slotID {
		return nil, common.ErrConflict
	}

	slot, err := slotSvc.GetSlotByID(slotID)
	if err != nil {
		return nil, err
	}

	view, err := s.EnsureRuntimeView(packageID)
	if err != nil {
		return nil, err
	}

	relation.Status = runtimeModel.StatusEnding
	if err = runtimeSvc.UpdateRelation(relation); err != nil {
		return nil, err
	}

	slot.Status = slotModel.StatusWaiting
	slot.CurrentPackageID = nil
	slot.CurrentRunID = nil
	slot.LastError = nil
	if err = slotSvc.UpdateSlot(slot); err != nil {
		return nil, err
	}

	view.RuntimeStatus = model.StatusStopped
	view.CurrentRunID = nil
	view.CurrentSlotID = nil
	if err = s.UpdateRuntimeView(view); err != nil {
		return nil, err
	}

	if err = runtimeSvc.DeleteRelationByRunID(relation.RunID); err != nil {
		return nil, err
	}

	// ******** 平台端接口接入点 10：
	// 当平台端后续提供“stop 成功后的运行结束回告、slot 释放登记、包状态同步”接口时，
	// 直接在这里调用，保证 package/slot/runtime relation 三层已经先在 Client 本机完成收口。
	result, err := s.GetByPackageID(packageID)
	if err != nil {
		return nil, err
	}
	if err = platformService.GetPackageRuntimeSync().AfterStopPackage(result); err != nil {
		return nil, err
	}
	return result, nil
}

func buildRunID(packageID string) string {
	return fmt.Sprintf("%s-%d", packageID, time.Now().UnixNano())
}

// DeleteRuntimeView 删除环境包在 Client 本机的当前运行视图。
//
// 这次先把正式 `DELETE /browser-envs/{envId}/package` 的最小本机事实删除能力接起来：
// - 只删除本地 SQLite 中的当前运行视图摘要；
// - 不在这里伪造目录、备份包或浏览器资产删除；
// - 后续真正接环境包资产目录时，仍然从 BrowserEnv 正式删除链路往这里复用。
func (s *Service) DeleteRuntimeView(packageID string) error {
	return packageDao.NewDeleteModelHandler().DeleteRuntimeViewByPackageID(strings.TrimSpace(packageID))
}
