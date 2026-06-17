package Platform

import (
	packageModel "private_browser_client/Models/Package"
)

// PackageRuntimeSync 定义 package run/stop 与平台端对接的统一边界。
//
// 当前先抽接口，不接真实平台 API。
// 这样后续你只要把平台端 run/stop 相关接口告诉我，我们直接在这个服务实现里补齐即可。
type PackageRuntimeSync interface {
	BeforeRunPackage(packageID string, slotID string) error
	AfterRunPackage(view *packageModel.RuntimeView) error
	BeforeStopPackage(packageID string, slotID string) error
	AfterStopPackage(view *packageModel.RuntimeView) error
}

var packageRuntimeSync PackageRuntimeSync = NewNoopPackageRuntimeSync()

func SetPackageRuntimeSync(sync PackageRuntimeSync) {
	if sync == nil {
		packageRuntimeSync = NewNoopPackageRuntimeSync()
		return
	}
	packageRuntimeSync = sync
}

func GetPackageRuntimeSync() PackageRuntimeSync {
	return packageRuntimeSync
}
