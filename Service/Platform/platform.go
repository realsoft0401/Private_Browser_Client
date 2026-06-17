package Platform

import (
	slotModel "private_browser_client/Models/Slot"
)

// SlotLifecycleSync 定义 slot 生命周期与平台端对接的统一边界。
//
// 设计来源：
// - 你已经明确要求，后续平台端接口就绪后，希望直接告诉接口，不想再回头到各个业务文件里找接入点；
// - 因此这里先把平台端的接入边界单独抽成 Service，后续只需要替换实现，不需要改业务主流程结构。
//
// 职责边界：
// - 这里只负责“Client 需要在什么时机与平台端沟通”；
// - 不负责 slot 本机状态机，不负责 Repository 持久化，不负责 HTTP 协议解析；
// - 业务动作仍然由 Slot/Package Service 驱动，这里只是被调用的外部同步边界。
type SlotLifecycleSync interface {
	BeforeCreateSlot(slotID string) error
	AfterCreateSlot(slot *slotModel.Slot) error
	BeforeDestroySlot(slotID string) error
	AfterDestroySlot(slotID string) error
	BeforeReinitSlot(slotID string) error
	AfterReinitSlot(slot *slotModel.Slot) error
}

var slotLifecycleSync SlotLifecycleSync = NewNoopSlotLifecycleSync()

// SetSlotLifecycleSync 允许后续把 noop 实现替换成真实平台接口实现。
//
// ******** 平台端接口最终接入点：
// 等后续平台端把真实 API 提供出来后，直接新增一个真实实现并在初始化阶段调用这里替换。
// 当前默认保留 noop，是因为这期已经收口：create-slot 暂时不受平台配额限制，可无限创建。
func SetSlotLifecycleSync(sync SlotLifecycleSync) {
	if sync == nil {
		slotLifecycleSync = NewNoopSlotLifecycleSync()
		return
	}
	slotLifecycleSync = sync
}

// GetSlotLifecycleSync 返回当前启用的平台同步实现。
//
// 当前默认实现是 noop，也就是：
// - create-slot 一律放行
// - destroy-slot/reinit-slot 不做平台端前后置通知
// 后续切平台真实实现时，业务层不需要改调用点。
func GetSlotLifecycleSync() SlotLifecycleSync {
	return slotLifecycleSync
}
