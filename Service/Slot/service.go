package Slot

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	slotDao "private_browser_client/Dao/Slot"
	model "private_browser_client/Models/Slot"
	common "private_browser_client/Repository/Common"
	platformService "private_browser_client/Service/Platform"
	slotRuntimeService "private_browser_client/Service/SlotRuntime"
)

type Service struct{}

var slotIDPattern = regexp.MustCompile(`^slot\d{3}$`)

// NewService 创建 slot 业务服务。
//
// 新模型里 slot 已经是一等资源对象，所以 Service/Slot 会成为后续很重要的一层：
// - create-slot
// - destroy-slot
// - reinit-slot
// - slot 当前态更新
//
// 当前阶段先把业务入口收口到这里，后续再逐步接容器初始化、资源清理和 SSE 上报。
func NewService() *Service {
	return &Service{}
}

// CreateSlot 创建本机 slot 当前态记录。
//
// 这一步现在只先完成“本机资源位事实落库前的业务入口”。
// 真正的容器初始化后续仍会补到这里，但不会绕过这个 Service 直接写 Repository。
//
// ******** 平台端接口接入点 1：
// 当后续平台端已经提供“slot 数量配额/是否允许创建”的正式接口后，
// 必须优先在这里调用平台端放行接口，再继续创建本机 slot。
// 这层是最合适的接入点，因为它位于 HTTP 入参校验之后、Repository 写入之前，
// 可以避免平台未放行时本机已经先落库，导致 Client 与平台事实不一致。
func (s *Service) CreateSlot(slotID string) (*model.Slot, error) {
	slotID = strings.TrimSpace(slotID)
	if slotID == "" {
		return nil, errors.New("slotId 不能为空")
	}
	if err := validateSlotIDFormat(slotID); err != nil {
		return nil, err
	}
	if err := platformService.GetSlotLifecycleSync().BeforeCreateSlot(slotID); err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	slot := &model.Slot{
		SlotID:        slotID,
		Status:        model.StatusLoading,
		InitializedAt: now,
		UpdatedAt:     now,
	}
	if err := slotDao.NewCreateModelHandler().CreateSlot(slot); err != nil {
		return nil, err
	}
	if err := slotRuntimeService.GetInitializer().Initialize(slot); err != nil {
		slot.LastError = optionalString(err.Error())
		_ = slotDao.NewDeleteModelHandler().DeleteSlotByID(slotID)
		return nil, err
	}
	slot.Status = model.StatusWaiting
	if err := s.UpdateSlot(slot); err != nil {
		_ = slotRuntimeService.GetInitializer().Destroy(slot)
		_ = slotDao.NewDeleteModelHandler().DeleteSlotByID(slotID)
		return nil, err
	}

	// ******** 平台端接口接入点 2：
	// 当后续平台端已经提供“create-slot 成功后登记/回告”的正式接口后，
	// 直接在这里调用平台端同步当前 slot 创建结果。
	// 当前先不接，是因为这期需求已经明确：暂时不处理平台端限制，只先做 Client 本机能力。
	if err := platformService.GetSlotLifecycleSync().AfterCreateSlot(slot); err != nil {
		return nil, err
	}
	return slot, nil
}

// GetSlotByID 查询当前 slot 状态。
func (s *Service) GetSlotByID(slotID string) (*model.Slot, error) {
	return slotDao.NewRuntimeModelHandler().GetSlotByID(strings.TrimSpace(slotID))
}

// ListSlots 返回当前进程内维护的全部 slot 当前态。
//
// 这里先返回当前态快照，方便 Node Server 或开发阶段页面确认“本机有哪些可分配槽位”。
// 等 SQLite 接回后，这个接口仍应保持“列出当前资源位事实”这个职责，不要混进历史审计。
func (s *Service) ListSlots() ([]*model.Slot, error) {
	return slotDao.NewRuntimeModelHandler().ListSlots()
}

// UpdateSlot 更新 slot 当前态。
func (s *Service) UpdateSlot(slot *model.Slot) error {
	if slot != nil {
		slot.UpdatedAt = time.Now().Unix()
	}
	return slotDao.NewRuntimeModelHandler().UpdateSlot(slot)
}

// DeleteSlotByID 删除本机 slot 当前态记录。
//
// 当前阶段只先建立动作入口；后续接真实销毁流程时，仍然从这里往下串
// “检查运行关系 -> 必要时强制结束 -> 删除 slot -> 容器清理”。
//
// ******** 平台端接口接入点 3：
// 当后续平台端已经提供“destroy-slot 前是否允许销毁 / 是否需要先做中心校验”的接口后，
// 应优先在这个 Service 动作链路里接入，而不是在 HTTP 层临时拼接。
//
// ******** 平台端接口接入点 4：
// 当后续平台端已经提供“destroy-slot 成功后的回告/登记”接口后，
// 也应在本机删除成功后从这层统一回传，保证平台与 Client 的 slot 事实同步。
func (s *Service) DeleteSlotByID(slotID string) error {
	slotID = strings.TrimSpace(slotID)
	slot, err := s.GetSlotByID(slotID)
	if err != nil {
		return err
	}
	if err := platformService.GetSlotLifecycleSync().BeforeDestroySlot(slotID); err != nil {
		return err
	}
	if err := slotRuntimeService.GetInitializer().Destroy(slot); err != nil {
		return err
	}
	if err := slotDao.NewDeleteModelHandler().DeleteSlotByID(slotID); err != nil {
		return err
	}
	return platformService.GetSlotLifecycleSync().AfterDestroySlot(slotID)
}

// ReinitSlot 把 slot 收回到 waiting 初始态。
//
// 根据前面的需求收口，slot 卡住时不是直接报废，而是由 Node Server 触发 Client 重初始化。
// 当前骨架阶段先收口成“只有已经脱离运行关系的 slot 才允许重置”，
// 避免把仍在占用中的 slot 静默改回 waiting。
//
// ******** 平台端接口接入点 5：
// 如果后续平台端要求“reinit-slot 之前先记录中心重置动作”或“重置完成后回写中心状态”，
// 统一从这里接，不要把平台端回调散落到 Node 临时脚本或 HTTP handler 里。
func (s *Service) ReinitSlot(slotID string) (*model.Slot, error) {
	slotID = strings.TrimSpace(slotID)
	if err := platformService.GetSlotLifecycleSync().BeforeReinitSlot(slotID); err != nil {
		return nil, err
	}

	slot, err := s.GetSlotByID(slotID)
	if err != nil {
		return nil, err
	}

	if slot.Status == model.StatusOccupied || slot.Status == model.StatusLoading || slot.Status == model.StatusReleasing {
		return nil, common.ErrConflict
	}

	slot.Status = model.StatusWaiting
	slot.CurrentPackageID = nil
	slot.CurrentRunID = nil
	slot.LastError = nil
	slot.UpdatedAt = time.Now().Unix()
	if err = slotRuntimeService.GetInitializer().Reinitialize(slot); err != nil {
		return nil, err
	}
	if err = s.UpdateSlot(slot); err != nil {
		return nil, err
	}

	// ******** 平台端接口接入点 6：
	// 当平台端后续提供“slot reinit 成功后同步 waiting 状态”的正式接口时，
	// 直接在这里调用，保持 Client 本机状态成功和平台侧同步动作在同一条主链路上。
	if err = platformService.GetSlotLifecycleSync().AfterReinitSlot(slot); err != nil {
		return nil, err
	}
	return slot, nil
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// validateSlotIDFormat 统一收口 slot 命名格式。
//
// 设计来源：
// - 当前资源位已经确定要统一成固定编号口径，避免历史阶段残留出 `slot-1`、`slot-e2e-*` 这类多套命名；
// - slot 不只是展示字段，还会进入容器名、端口偏移、WebVNC 地址和后续平台排障口径；
// - 因此格式必须在 Service 入口就收紧，不能等到 Repository 或前端页面再各自猜测。
//
// 当前固定规则：
// - 前缀固定 `slot`
// - 后缀固定 3 位数字
// - 典型值：`slot001`、`slot002`、`slot120`
//
// 维护边界：
// - 这里只负责格式约束，不负责判断是否重复；
// - 是否允许未来扩成 4 位或平台统一分配规则，应从这里统一改，不要把正则散到多个 handler。
func validateSlotIDFormat(slotID string) error {
	if !slotIDPattern.MatchString(slotID) {
		return fmt.Errorf("slotId 格式必须是 slot001 这种 3 位编号形式")
	}
	return nil
}
