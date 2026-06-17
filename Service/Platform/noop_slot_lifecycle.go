package Platform

import (
	slotModel "private_browser_client/Models/Slot"
)

// NoopSlotLifecycleSync 是当前阶段的平台端空实现。
//
// 设计来源：
// - 当前需求已经明确：平台端接口还没接，create-slot 先允许无限创建；
// - 但为了后续能平滑接入平台配额、登记、回告，这里先把调用面固定下来。
//
// 职责边界：
// - 当前所有方法都直接放行，不访问任何平台 API；
// - 这是临时实现，不代表最终平台策略；
// - 后续平台接口就绪后，应新增真实实现替换本文件，而不是把平台调用重新散回业务代码。
type NoopSlotLifecycleSync struct{}

func NewNoopSlotLifecycleSync() *NoopSlotLifecycleSync {
	return &NoopSlotLifecycleSync{}
}

func (s *NoopSlotLifecycleSync) BeforeCreateSlot(slotID string) error {
	_ = slotID
	return nil
}

func (s *NoopSlotLifecycleSync) AfterCreateSlot(slot *slotModel.Slot) error {
	_ = slot
	return nil
}

func (s *NoopSlotLifecycleSync) BeforeDestroySlot(slotID string) error {
	_ = slotID
	return nil
}

func (s *NoopSlotLifecycleSync) AfterDestroySlot(slotID string) error {
	_ = slotID
	return nil
}

func (s *NoopSlotLifecycleSync) BeforeReinitSlot(slotID string) error {
	_ = slotID
	return nil
}

func (s *NoopSlotLifecycleSync) AfterReinitSlot(slot *slotModel.Slot) error {
	_ = slot
	return nil
}
