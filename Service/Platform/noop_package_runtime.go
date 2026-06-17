package Platform

import (
	packageModel "private_browser_client/Models/Package"
)

// NoopPackageRuntimeSync 是 run/stop 当前阶段的平台空实现。
//
// 当前阶段说明：
// - run/stop 先只做 Client 本机状态机；
// - 不做平台任务登记，不做平台放行，不做平台结果回告；
// - 后续平台接口就绪后，直接替换这里的实现即可。
type NoopPackageRuntimeSync struct{}

func NewNoopPackageRuntimeSync() *NoopPackageRuntimeSync {
	return &NoopPackageRuntimeSync{}
}

func (s *NoopPackageRuntimeSync) BeforeRunPackage(packageID string, slotID string) error {
	_, _ = packageID, slotID
	return nil
}

func (s *NoopPackageRuntimeSync) AfterRunPackage(view *packageModel.RuntimeView) error {
	_ = view
	return nil
}

func (s *NoopPackageRuntimeSync) BeforeStopPackage(packageID string, slotID string) error {
	_, _ = packageID, slotID
	return nil
}

func (s *NoopPackageRuntimeSync) AfterStopPackage(view *packageModel.RuntimeView) error {
	_ = view
	return nil
}
