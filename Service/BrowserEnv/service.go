package BrowserEnv

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	packageModel "private_browser_client/Models/Package"
	slotModel "private_browser_client/Models/Slot"
	taskModel "private_browser_client/Models/Task"
	common "private_browser_client/Repository/Common"
	edgeService "private_browser_client/Service/Edge"
	packageService "private_browser_client/Service/Package"
	runtimeService "private_browser_client/Service/Runtime"
	slotService "private_browser_client/Service/Slot"
	slotRuntimeService "private_browser_client/Service/SlotRuntime"
	taskService "private_browser_client/Service/Task"
	"private_browser_client/Settings"
)

type Service struct{}

// NewService 创建正式 browser-env 协议服务。
//
// 这层的职责不是替代底层 Package/Runtime/Slot Service，
// 而是把已经收紧好的正式 `browser-envs/*` 协议统一映射到现有本机能力：
// - 统一 envId 命名；
// - 统一 taskId/eventsUrl 协议；
// - 统一正式状态和值域；
// - 逐步承接后续真正的环境包资产能力。
func NewService() *Service {
	return &Service{}
}

// Create 在当前 Client 本机创建一份正式 browser-env 资产。
//
// 设计来源：
// - 当前需求已经把“包是包，容器是容器”收口清楚，create 只负责资产建立，不负责 run；
// - 用户明确要求目录结构、profile/binding/container 文件与 old 保持一致；
// - 这一步必须同步写入 SQLite browser_envs 索引，否则后续 lifecycle API 没有稳定事实源。
//
// 职责边界：
// - 负责参数校验、envId/bindingId/envSequence/端口生成、目录和文件落盘、SQLite 索引写入；
// - 负责补一条 package 当前运行视图的 created 记录，便于正式 run/stop 直接复用；
// - 不负责启动容器，不负责平台配额，不负责中心身份登记。
func (s *Service) Create(request *model.CreateBrowserEnvRequest) (*model.CreateBrowserEnvResponse, error) {
	normalized, err := normalizeCreateRequest(request)
	if err != nil {
		return nil, err
	}

	createEnvMu.Lock()
	defer createEnvMu.Unlock()

	ctx, err := newCreateContext(normalized)
	if err != nil {
		return nil, err
	}
	if err = ensureEnvPathAvailable(ctx.AbsoluteEnvPath); err != nil {
		return nil, err
	}
	if err = createEnvDirectories(ctx.AbsoluteEnvPath); err != nil {
		return nil, internalError(err.Error())
	}

	created := false
	defer cleanupPartialEnvPackage(ctx.AbsoluteEnvPath, &created)

	files := buildPackageFiles(ctx)
	if err = writeEnvPackageFiles(ctx.AbsoluteEnvPath, ctx.Paths, files); err != nil {
		return nil, internalError(err.Error())
	}
	if err = browserEnvDao.NewCreateModelHandler().CreateBrowserEnvIndex(buildBrowserEnvIndex(ctx, files)); err != nil {
		if errors.Is(err, common.ErrDuplicate) {
			return nil, conflictError("envId 已存在")
		}
		return nil, internalError(err.Error())
	}
	if _, err = packageService.NewService().CreateRuntimeView(ctx.EnvID); err != nil && !errors.Is(err, common.ErrDuplicate) {
		return nil, internalError(err.Error())
	}

	created = true
	return buildCreateResponse(ctx), nil
}

// Run 按正式 browser-env run 协议创建长链路任务。
func (s *Service) Run(envID string, request model.RunRequest) (*model.TaskAcceptedResponse, error) {
	envID = strings.TrimSpace(envID)
	request.SlotID = strings.TrimSpace(request.SlotID)
	if envID == "" || request.SlotID == "" {
		return nil, errors.New("envId 和 slotId 不能为空")
	}

	taskID := taskService.GetService().CreateTask("browser_env_run", "browser_env", envID)
	go s.executeRun(taskID, envID, request)
	return &model.TaskAcceptedResponse{
		TaskID:       taskID,
		TaskType:     "browser_env_run",
		ResourceType: "browser_env",
		ResourceID:   envID,
		EventsURL:    fmt.Sprintf("/api/v1/edge/tasks/%s/events", taskID),
	}, nil
}

// Stop 按正式 browser-env stop 协议同步收口当前运行态。
func (s *Service) Stop(envID string, request model.StopRequest) (*model.StopResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, errors.New("envId 不能为空")
	}

	pkgSvc := packageService.NewService()
	runtimeSvc := runtimeService.NewService()

	relation, err := runtimeSvc.GetByPackageID(envID)
	if err != nil {
		if !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}

		view, ensureErr := pkgSvc.EnsureRuntimeView(envID)
		if ensureErr != nil {
			return nil, ensureErr
		}
		view.RuntimeStatus = packageModel.StatusStopped
		view.CurrentRunID = nil
		view.CurrentSlotID = nil
		view.LastError = nil
		if updateErr := pkgSvc.UpdateRuntimeView(view); updateErr != nil {
			return nil, updateErr
		}
		_ = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
			EnvID:           envID,
			Status:          model.BrowserEnvStatusStopped,
			ContainerID:     nil,
			ContainerName:   nil,
			ContainerStatus: model.ContainerStatusMissing,
			MonitorStatus:   model.MonitorStatusUnknown,
			LastError:       nil,
			UpdatedAt:       time.Now().Unix(),
			LastStoppedAt:   optionalInt64(time.Now().Unix()),
			LastCheckedAt:   optionalInt64(time.Now().Unix()),
		})
		return &model.StopResponse{
			EnvID:           envID,
			Status:          packageModel.StatusStopped,
			ContainerStatus: "missing",
			StoppedAt:       time.Now().Unix(),
		}, nil
	}

	stopTimeout := normalizeBrowserEnvStopTimeout(&request)
	if stopErr := gracefulStopBrowserEnvContainer(relation.SlotID, stopTimeout); stopErr != nil {
		return nil, stopErr
	}
	waitForBrowserStateFlush(stopTimeout)

	view, err := pkgSvc.StopPackage(envID, relation.SlotID)
	if err != nil {
		return nil, err
	}
	if err = resetSlotToBlankWaitingRuntime(relation.SlotID); err != nil {
		return nil, err
	}
	_ = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
		EnvID:           envID,
		Status:          model.BrowserEnvStatusStopped,
		ContainerID:     nil,
		ContainerName:   nil,
		ContainerStatus: model.ContainerStatusMissing,
		MonitorStatus:   model.MonitorStatusUnknown,
		LastError:       nil,
		UpdatedAt:       time.Now().Unix(),
		LastStoppedAt:   optionalInt64(derefTime(view.LastStopAt)),
		LastCheckedAt:   optionalInt64(time.Now().Unix()),
	})
	return &model.StopResponse{
		EnvID:           envID,
		Status:          view.RuntimeStatus,
		ContainerStatus: "missing",
		StoppedAt:       derefTime(view.LastStopAt),
	}, nil
}

// gracefulStopBrowserEnvContainer 在 slot 重初始化前先优雅停止真实运行容器。
//
// 设计来源：
// - 当前 browser-env run 会把 slot 常驻容器替换成真正加载环境包资产的浏览器容器；
// - 如果 stop 仍然直接 Reinitialize，相当于先 force remove 再建占位容器，Chromium 来不及把 TK 登录态刷回 profile；
// - 因此这里先按 slot 当前容器事实做 Docker stop，再进入 package/slot 状态收口。
//
// 职责边界：
// - 这里只做容器停止，不负责 SQLite 和包状态更新；
// - 容器不存在时按已停止处理，避免人工删容器后 stop 无法收口。
func gracefulStopBrowserEnvContainer(slotID string, timeoutSeconds int) error {
	slot, err := slotService.NewService().GetSlotByID(strings.TrimSpace(slotID))
	if err != nil {
		return err
	}

	containerID := ""
	if slot.ContainerID != nil {
		containerID = strings.TrimSpace(*slot.ContainerID)
	}
	containerName := ""
	if slot.ContainerName != nil {
		containerName = strings.TrimSpace(*slot.ContainerName)
	}
	target := firstNonEmpty(containerID, containerName)
	if target == "" {
		return nil
	}

	action, err := edgeService.NewEdgeService().StopDockerContainer(target, &edgeModel.ContainerActionRequest{
		TimeoutSeconds: &timeoutSeconds,
	})
	if err != nil {
		if isDockerContainerNotFound(err) {
			return nil
		}
		return err
	}
	if action == nil {
		return nil
	}
	return nil
}

// normalizeBrowserEnvStopTimeout 统一 browser-env stop 的 Docker 等待秒数。
//
// 正式 browser-env stop 当前是同步接口，不适合无限等待；
// 但也不能回退成“立即删容器”，否则真实登录态回写会再次受损。
func normalizeBrowserEnvStopTimeout(request *model.StopRequest) int {
	if request == nil || request.TimeoutSeconds <= 0 {
		return 10
	}
	if request.TimeoutSeconds > 3600 {
		return 3600
	}
	return request.TimeoutSeconds
}

// waitForBrowserStateFlush 在 Docker stop 返回后给浏览器文件系统一个很短的落盘缓冲。
//
// 设计来源：
// - Docker stop 表示主进程已退出，但 Docker Desktop/macOS bind mount 在极短时间内仍可能有文件事件收尾；
// - 这次 TikTok 回归里我们要优先保证 package 完整性，而不是追求 stop 返回的极限速度；
// - 因此在 stop 完成和 slot 重建之间保留一小段固定缓冲，降低 Cookies/IndexedDB 最后写入被截断的概率。
func waitForBrowserStateFlush(timeoutSeconds int) {
	waitSeconds := 2
	if timeoutSeconds > 0 && timeoutSeconds < waitSeconds {
		waitSeconds = timeoutSeconds
	}
	if waitSeconds <= 0 {
		return
	}
	time.Sleep(time.Duration(waitSeconds) * time.Second)
}

// firstNonEmpty 选择 stop 链路要使用的容器标识。
//
// Docker stop 允许使用 containerId 或 containerName；
// 这里保持“ID 优先、名称兜底”，避免 slot 恢复链路在旧数据场景下失去停止能力。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// isDockerContainerNotFound 识别 Docker 404 的幂等停止场景。
//
// 用户可能已经手工删过 slot 容器；这时 stop 应继续把包状态收口为 stopped，
// 而不是因为容器不存在就把正式生命周期卡死。
func isDockerContainerNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "status=404")
}

// resetSlotToBlankWaitingRuntime 把 slot 收回到“空白 waiting 容器”。
//
// 设计来源：
// - 当前需求已经明确：stop / ending / backup 后，slot 必须恢复成不挂任何包资产的基础服务容器；
// - waiting 不是“上一个包停在原地”，而是“容器与包彻底断绝关系后的空白资源位”；
// - 因此把重置动作收口成统一函数，避免不同生命周期路径各自处理而留下残影。
//
// 职责边界：
// - 负责把 slot 先标记为 releasing，再重建空白容器，最后收口成 waiting；
// - 负责清空 currentPackageId/currentRunId/lastError；
// - 不负责 package/runtime relation 的主状态更新。
func resetSlotToBlankWaitingRuntime(slotID string) error {
	slotSvc := slotService.NewService()
	slot, err := slotSvc.GetSlotByID(strings.TrimSpace(slotID))
	if err != nil {
		return err
	}

	slot.Status = slotModel.StatusReleasing
	slot.LastError = nil
	if err = slotSvc.UpdateSlot(slot); err != nil {
		return err
	}

	if err = slotRuntimeService.GetInitializer().Reinitialize(slot); err != nil {
		slot.LastError = optionalString(err.Error())
		_ = slotSvc.UpdateSlot(slot)
		return err
	}

	slot.Status = slotModel.StatusWaiting
	slot.CurrentPackageID = nil
	slot.CurrentRunID = nil
	slot.LastError = nil
	return slotSvc.UpdateSlot(slot)
}

// detachEnvFromAnyResidualSlots 在 backup 成功前兜底清理残留 slot 绑定。
//
// 正常链路里，env 在 backup 前应该已经先 stop，所以 slot 已经回到 waiting。
// 这里再补一层扫描，是为了守住“backup 后容器与包彻底断绝关系”的原则：
// 即使历史数据、人工操作或中途异常留下 currentPackageId 残影，也必须在 backup 成功前清干净。
func detachEnvFromAnyResidualSlots(envID string) error {
	slots, err := slotService.NewService().ListSlots()
	if err != nil {
		return err
	}
	for _, slot := range slots {
		if slot == nil || slot.CurrentPackageID == nil {
			continue
		}
		if strings.TrimSpace(*slot.CurrentPackageID) != strings.TrimSpace(envID) {
			continue
		}
		if err = resetSlotToBlankWaitingRuntime(slot.SlotID); err != nil {
			return err
		}
	}
	return nil
}

// DeletePackage 先实现最小正式删除链路：
// - 非运行态时允许删除本地 SQLite 运行视图；
// - running 时统一返回生命周期冲突；
// - 后续真正接目录、backup、browser-data 资产删除时继续复用这条正式入口。
func (s *Service) DeletePackage(envID string) (*model.TaskAcceptedResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, errors.New("envId 不能为空")
	}

	if _, err := runtimeService.NewService().GetByPackageID(envID); err == nil {
		return nil, common.ErrConflict
	} else if !errors.Is(err, common.ErrNotFound) {
		return nil, err
	}

	taskID := taskService.GetService().CreateTask("browser_env_delete_package", "browser_env", envID)
	go s.executeDelete(taskID, envID)
	return &model.TaskAcceptedResponse{
		TaskID:       taskID,
		TaskType:     "browser_env_delete_package",
		ResourceType: "browser_env",
		ResourceID:   envID,
		EventsURL:    fmt.Sprintf("/api/v1/edge/tasks/%s/events", taskID),
	}, nil
}

// UpdateProxy 修改环境包代理配置并把运行保护摘要重置为 pending。
func (s *Service) UpdateProxy(envID string, request *model.UpdateBrowserEnvProxyRequest) (*model.UpdateBrowserEnvProxyResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}
	if request == nil {
		return nil, invalidError("请求参数不能为空")
	}

	index, err := loadBrowserEnvIndexOrFail(envID)
	if err != nil {
		return nil, err
	}
	switch index.Status {
	case model.BrowserEnvStatusDeleted:
		return nil, conflictError("环境包已删除，不能修改配置")
	case model.BrowserEnvStatusBackedUp:
		return nil, conflictError("环境包当前只有备份包，请先 restore 后再修改配置")
	case model.BrowserEnvStatusError:
		return nil, conflictError("环境包处于 error，请先 revalidate 后再修改配置")
	}

	pkg, err := loadPackage(index)
	if err != nil {
		return nil, err
	}

	enabled := pkg.Profile.Proxy.Enabled
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	proxyType := pkg.Profile.Proxy.Type
	if request.Type != nil {
		proxyType = strings.TrimSpace(*request.Type)
	}
	config := pkg.ProxyConfig
	if request.ConfigBase64 != nil {
		decoded, err := decodeProxyConfigBase64(*request.ConfigBase64)
		if err != nil {
			return nil, err
		}
		config = decoded
	}
	if enabled {
		if proxyType == "" {
			proxyType = "clash"
		}
		if proxyType != "clash" {
			return nil, invalidError("proxy.type 当前仅支持 clash")
		}
		if request.Mode != nil {
			mode, err := normalizeClashMode(*request.Mode)
			if err != nil {
				return nil, err
			}
			updated, _, err := replaceClashMode(config, mode)
			if err != nil {
				return nil, err
			}
			config = updated
		}
		if strings.TrimSpace(config) == "" {
			return nil, invalidError("proxy.enabled=true 时 proxy.configBase64 不能为空")
		}
	} else {
		proxyType = ""
		config = ""
	}

	now := time.Now().Unix()
	pkg.Profile.Proxy.Enabled = enabled
	pkg.Profile.Proxy.Type = proxyType
	pkg.Profile.Metadata.UpdatedAt = now
	pkg.Binding.RuntimeProtection.TimezoneStatus = "pending"
	pkg.Binding.RuntimeProtection.RiskStatus = "pending"
	pkg.Binding.RuntimeProtection.AvailabilityStatus = "pending"
	pkg.Binding.RuntimeProtection.LastError = ""
	pkg.Binding.UpdatedAt = now

	if err = writePackageJSON(pkg.EnvPath, pkg.Profile.Paths.Profile, pkg.Profile); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(pkg.EnvPath, pkg.Profile.Paths.Binding, pkg.Binding); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageText(pkg.EnvPath, pkg.Profile.Paths.ProxyConfig, config); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writeTimezoneProbePending(pkg.EnvPath, pkg.Profile.Paths.ProxyRuntime); err != nil {
		return nil, internalError(err.Error())
	}
	if err = browserEnvDao.NewConfigModelHandler().UpdateBrowserEnvConfig(&model.BrowserEnvConfigUpdate{
		EnvID:     envID,
		Status:    index.Status,
		LastError: index.LastError,
		UpdatedAt: now,
	}); err != nil {
		return nil, internalError(err.Error())
	}
	return &model.UpdateBrowserEnvProxyResponse{
		EnvID:                   envID,
		RestartQueued:           false,
		RuntimeProtectionStatus: "pending",
		ProxyRuntimeStatus:      "pending",
	}, nil
}

// Backup 为环境包创建正式备份任务。
func (s *Service) Backup(envID string) (*model.TaskAcceptedResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}
	if _, err := runtimeService.NewService().GetByPackageID(envID); err == nil {
		return nil, conflictError("环境包正在运行，请先停止后再备份")
	} else if !errors.Is(err, common.ErrNotFound) {
		return nil, err
	}
	taskID := taskService.GetService().CreateTask("browser_env_backup", "browser_env", envID)
	go s.executeBackup(taskID, envID)
	return acceptedTask(taskID, "browser_env_backup", envID), nil
}

// Restore 为环境包创建恢复任务。
func (s *Service) Restore(envID string) (*model.TaskAcceptedResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}
	taskID := taskService.GetService().CreateTask("browser_env_restore", "browser_env", envID)
	go s.executeRestore(taskID, envID)
	return acceptedTask(taskID, "browser_env_restore", envID), nil
}

// Revalidate 为异常环境包创建受控重新校验任务。
func (s *Service) Revalidate(envID string) (*model.TaskAcceptedResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}
	taskID := taskService.GetService().CreateTask("browser_env_revalidate", "browser_env", envID)
	go s.executeRevalidate(taskID, envID)
	return acceptedTask(taskID, "browser_env_revalidate", envID), nil
}

// ImportPackage 先把上传文件保存为临时归档，再异步导入为正式环境包。
func (s *Service) ImportPackage(file io.Reader, originalName string) (*model.TaskAcceptedResponse, error) {
	if file == nil {
		return nil, invalidError("导入文件不能为空")
	}
	if strings.TrimSpace(originalName) == "" {
		return nil, invalidError("导入文件名不能为空")
	}
	temp, err := os.CreateTemp("", "private-browser-import-*.tar.gz")
	if err != nil {
		return nil, internalError(fmt.Sprintf("创建导入临时文件失败: %v", err))
	}
	tempPath := temp.Name()
	if _, err = io.Copy(temp, io.LimitReader(file, maxImportPackageBytes)); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return nil, internalError(fmt.Sprintf("写入导入临时文件失败: %v", err))
	}
	if err = temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return nil, internalError(fmt.Sprintf("关闭导入临时文件失败: %v", err))
	}

	taskID := taskService.GetService().CreateTask("browser_env_import_package", "browser_env", "")
	go s.executeImport(taskID, tempPath)
	return &model.TaskAcceptedResponse{
		TaskID:       taskID,
		TaskType:     "browser_env_import_package",
		ResourceType: "browser_env",
		ResourceID:   "",
		EventsURL:    fmt.Sprintf("/api/v1/edge/tasks/%s/events", taskID),
	}, nil
}

func (s *Service) executeRun(taskID string, envID string, request model.RunRequest) {
	publisher := taskService.GetService()
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_run", envID, request.SlotID, "validate_env_index", taskModel.StatusQueued, "task accepted", "", ""))
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_run", envID, request.SlotID, "start_container", taskModel.StatusRunning, "starting browser env", "", ""))

	index, err := loadBrowserEnvIndexOrFail(envID)
	if err != nil {
		_ = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
			EnvID:           envID,
			Status:          model.BrowserEnvStatusError,
			ContainerStatus: model.ContainerStatusError,
			MonitorStatus:   model.MonitorStatusUnknown,
			LastError:       optionalString(err.Error()),
			UpdatedAt:       time.Now().Unix(),
			LastCheckedAt:   optionalInt64(time.Now().Unix()),
		})
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_run", envID, request.SlotID, "load_env_index_failed", taskModel.StatusFailed, "browser env run failed", err.Error(), suggestionForError(err)))
		return
	}
	pkg, err := validateAtomicPackage(index)
	if err != nil {
		_ = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
			EnvID:           envID,
			Status:          model.BrowserEnvStatusError,
			ContainerStatus: model.ContainerStatusError,
			MonitorStatus:   model.MonitorStatusUnknown,
			LastError:       optionalString(err.Error()),
			UpdatedAt:       time.Now().Unix(),
			LastCheckedAt:   optionalInt64(time.Now().Unix()),
		})
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_run", envID, request.SlotID, "validate_atomic_materials_failed", taskModel.StatusFailed, "browser env run failed", err.Error(), suggestionForError(err)))
		return
	}
	slot, err := slotService.NewService().GetSlotByID(request.SlotID)
	if err != nil {
		_ = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
			EnvID:           envID,
			Status:          model.BrowserEnvStatusError,
			ContainerStatus: model.ContainerStatusError,
			MonitorStatus:   model.MonitorStatusUnknown,
			LastError:       optionalString(err.Error()),
			UpdatedAt:       time.Now().Unix(),
			LastCheckedAt:   optionalInt64(time.Now().Unix()),
		})
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_run", envID, request.SlotID, "load_slot_failed", taskModel.StatusFailed, "browser env run failed", err.Error(), suggestionForError(err)))
		return
	}
	created, err := slotRuntimeRebuilder(slot, pkg)
	if err != nil {
		_ = slotRuntimeService.GetInitializer().Reinitialize(slot)
		_ = slotService.NewService().UpdateSlot(slot)
		_ = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
			EnvID:           envID,
			Status:          model.BrowserEnvStatusError,
			ContainerStatus: model.ContainerStatusError,
			MonitorStatus:   model.MonitorStatusUnknown,
			LastError:       optionalString(err.Error()),
			UpdatedAt:       time.Now().Unix(),
			LastCheckedAt:   optionalInt64(time.Now().Unix()),
		})
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_run", envID, request.SlotID, "prepare_slot_runtime_failed", taskModel.StatusFailed, "browser env run failed", err.Error(), suggestionForError(err)))
		return
	}
	if err = slotService.NewService().UpdateSlot(slot); err != nil {
		_ = maybeRemoveContainer(optionalString(created.ID))
		_ = slotRuntimeService.GetInitializer().Reinitialize(slot)
		_ = slotService.NewService().UpdateSlot(slot)
		_ = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
			EnvID:           envID,
			Status:          model.BrowserEnvStatusError,
			ContainerStatus: model.ContainerStatusError,
			MonitorStatus:   model.MonitorStatusUnknown,
			LastError:       optionalString(err.Error()),
			UpdatedAt:       time.Now().Unix(),
			LastCheckedAt:   optionalInt64(time.Now().Unix()),
		})
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_run", envID, request.SlotID, "update_slot_runtime_failed", taskModel.StatusFailed, "browser env run failed", err.Error(), suggestionForError(err)))
		return
	}

	result, err := packageService.NewService().RunPackage(envID, request.SlotID)
	if err != nil {
		_ = maybeRemoveContainer(slot.ContainerID)
		_ = slotRuntimeService.GetInitializer().Reinitialize(slot)
		_ = slotService.NewService().UpdateSlot(slot)
		_ = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
			EnvID:           envID,
			Status:          model.BrowserEnvStatusError,
			ContainerStatus: model.ContainerStatusError,
			MonitorStatus:   model.MonitorStatusUnknown,
			LastError:       optionalString(err.Error()),
			UpdatedAt:       time.Now().Unix(),
			LastCheckedAt:   optionalInt64(time.Now().Unix()),
		})
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_run", envID, request.SlotID, "finalize_failed", taskModel.StatusFailed, "browser env run failed", err.Error(), suggestionForError(err)))
		return
	}
	now := time.Now().Unix()
	_ = persistRunRuntimeSummary(pkg, slot, created.ID, now)
	_ = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
		EnvID:           envID,
		Status:          model.BrowserEnvStatusRunning,
		ContainerID:     optionalString(created.ID),
		ContainerName:   slot.ContainerName,
		ContainerStatus: model.ContainerStatusRunning,
		MonitorStatus:   model.MonitorStatusUnknown,
		LastError:       nil,
		UpdatedAt:       now,
		LastStartedAt:   optionalInt64(now),
		LastCheckedAt:   optionalInt64(now),
	})

	_ = publisher.PublishCompleted(taskID, newTaskEvent(taskModel.EventCompleted, taskID, "browser_env_run", envID, safeString(result.CurrentSlotID), "finalize_success", taskModel.StatusSuccess, "browser env is ready", "", ""))
}

func (s *Service) executeDelete(taskID string, envID string) {
	publisher := taskService.GetService()
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_delete_package", envID, "", "load_env_index", taskModel.StatusQueued, "task accepted", "", ""))
	index, err := loadBrowserEnvIndexOrFail(envID)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_delete_package", envID, "", "load_env_index_failed", taskModel.StatusFailed, "delete browser env package failed", err.Error(), suggestionForError(err)))
		return
	}
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_delete_package", envID, "", "remove_assets", taskModel.StatusRunning, "removing local browser env assets", "", ""))
	if err = maybeRemoveContainer(index.ContainerID); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_delete_package", envID, "", "remove_container_failed", taskModel.StatusFailed, "delete browser env package failed", err.Error(), suggestionForError(err)))
		return
	}
	if index.BackupPath != nil && strings.TrimSpace(*index.BackupPath) != "" {
		if backupAbs, backupErr := resolveManagedBackupPath(index); backupErr == nil {
			_ = os.RemoveAll(backupAbs)
		}
	}
	if envPath, pathErr := resolveManagedEnvPath(index); pathErr == nil {
		_ = os.RemoveAll(envPath)
	}
	if err = packageService.NewService().DeleteRuntimeView(envID); err != nil && !errors.Is(err, common.ErrNotFound) {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_delete_package", envID, "", "remove_runtime_view_failed", taskModel.StatusFailed, "delete browser env package failed", err.Error(), suggestionForError(err)))
		return
	}
	if err = browserEnvDao.NewDeleteModelHandler().DeleteBrowserEnvIndex(envID); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_delete_package", envID, "", "finalize_failed", taskModel.StatusFailed, "delete browser env package failed", err.Error(), suggestionForError(err)))
		return
	}

	_ = publisher.PublishCompleted(taskID, newTaskEvent(taskModel.EventCompleted, taskID, "browser_env_delete_package", envID, "", "finalize_success", taskModel.StatusSuccess, "browser env package deleted", "", ""))
}

func (s *Service) executeBackup(taskID string, envID string) {
	publisher := taskService.GetService()
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_backup", envID, "", "load_env_index", taskModel.StatusQueued, "task accepted", "", ""))
	index, err := loadBrowserEnvIndexOrFail(envID)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "load_env_index_failed", taskModel.StatusFailed, "backup failed", err.Error(), suggestionForError(err)))
		return
	}
	if index.Status == model.BrowserEnvStatusBackedUp {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "validate_status_failed", taskModel.StatusFailed, "backup failed", "环境包已经是备份状态", "restore before backup again"))
		return
	}
	pkg, err := validateAtomicPackage(index)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "validate_atomic_materials_failed", taskModel.StatusFailed, "backup failed", err.Error(), suggestionForError(err)))
		return
	}
	backupAbs, backupRel, err := managedBackupArchivePath(index)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "prepare_backup_path_failed", taskModel.StatusFailed, "backup failed", err.Error(), suggestionForError(err)))
		return
	}
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_backup", envID, "", "archive_package", taskModel.StatusRunning, "archiving browser env", "", ""))
	if err = createTarGzFromDirectory(pkg.EnvPath, index.EnvID, backupAbs); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "archive_package_failed", taskModel.StatusFailed, "backup failed", err.Error(), suggestionForError(err)))
		return
	}
	sum, err := fileSHA256(backupAbs)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "checksum_failed", taskModel.StatusFailed, "backup failed", err.Error(), suggestionForError(err)))
		return
	}
	stat, err := os.Stat(backupAbs)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "stat_backup_failed", taskModel.StatusFailed, "backup failed", err.Error(), suggestionForError(err)))
		return
	}
	_ = maybeRemoveContainer(index.ContainerID)
	if err = detachEnvFromAnyResidualSlots(envID); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "detach_slot_runtime_failed", taskModel.StatusFailed, "backup failed", err.Error(), suggestionForError(err)))
		return
	}
	if err = os.RemoveAll(pkg.EnvPath); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "remove_source_env_failed", taskModel.StatusFailed, "backup failed", err.Error(), suggestionForError(err)))
		return
	}
	now := time.Now().Unix()
	if err = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvBackupState(&model.BrowserEnvBackupStateUpdate{
		EnvID:           envID,
		Status:          model.BrowserEnvStatusBackedUp,
		ContainerID:     nil,
		ContainerName:   index.ContainerName,
		ContainerStatus: model.ContainerStatusMissing,
		MonitorStatus:   model.MonitorStatusUnknown,
		LastError:       nil,
		HasBrowserData:  false,
		BackupPath:      &backupRel,
		BackupChecksum:  &sum,
		BackupSize:      optionalInt64(stat.Size()),
		BackupAt:        &now,
		UpdatedAt:       now,
		LastStoppedAt:   &now,
		LastCheckedAt:   &now,
	}); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_backup", envID, "", "update_index_failed", taskModel.StatusFailed, "backup failed", err.Error(), suggestionForError(err)))
		return
	}
	_ = publisher.PublishCompleted(taskID, newTaskEvent(taskModel.EventCompleted, taskID, "browser_env_backup", envID, "", "finalize_success", taskModel.StatusSuccess, "browser env backed up", "", ""))
}

func (s *Service) executeRestore(taskID string, envID string) {
	publisher := taskService.GetService()
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_restore", envID, "", "load_env_index", taskModel.StatusQueued, "task accepted", "", ""))
	index, err := loadBrowserEnvIndexOrFail(envID)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "load_env_index_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	if index.Status != model.BrowserEnvStatusBackedUp {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "validate_status_failed", taskModel.StatusFailed, "restore failed", "环境包不是备份状态", "backup before restore is not required"))
		return
	}
	backupAbs, err := resolveManagedBackupPath(index)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "resolve_backup_path_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	if err = verifyBackupArchiveFile(index, backupAbs); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "verify_backup_archive_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	envPath, err := resolveManagedEnvPath(index)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "resolve_env_path_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	if err = ensureEnvPathAvailable(envPath); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "check_env_path_conflict_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	stagingRoot, err := os.MkdirTemp("", "private-browser-restore-*")
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "create_staging_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	defer os.RemoveAll(stagingRoot)
	file, err := os.Open(backupAbs)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "open_backup_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	defer file.Close()
	if err = extractImportTarGz(file, stagingRoot); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "extract_backup_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	root, err := findImportPackageRoot(stagingRoot)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "validate_archive_structure_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	if err = copyDirectory(root, envPath); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "promote_staging_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	_ = os.Remove(backupAbs)
	now := time.Now().Unix()
	if err = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvBackupState(&model.BrowserEnvBackupStateUpdate{
		EnvID:           envID,
		Status:          model.BrowserEnvStatusCreated,
		ContainerID:     nil,
		ContainerName:   index.ContainerName,
		ContainerStatus: model.ContainerStatusMissing,
		MonitorStatus:   model.MonitorStatusUnknown,
		LastError:       nil,
		HasBrowserData:  true,
		BackupPath:      nil,
		BackupChecksum:  nil,
		BackupSize:      nil,
		BackupAt:        nil,
		UpdatedAt:       now,
		LastRestoredAt:  &now,
		LastCheckedAt:   &now,
	}); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_restore", envID, "", "update_index_failed", taskModel.StatusFailed, "restore failed", err.Error(), suggestionForError(err)))
		return
	}
	_ = publisher.PublishCompleted(taskID, newTaskEvent(taskModel.EventCompleted, taskID, "browser_env_restore", envID, "", "finalize_success", taskModel.StatusSuccess, "browser env restored", "", ""))
}

func (s *Service) executeRevalidate(taskID string, envID string) {
	publisher := taskService.GetService()
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_revalidate", envID, "", "load_env_index", taskModel.StatusQueued, "task accepted", "", ""))
	index, err := loadBrowserEnvIndexOrFail(envID)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_revalidate", envID, "", "load_env_index_failed", taskModel.StatusFailed, "revalidate failed", err.Error(), suggestionForError(err)))
		return
	}
	if index.Status != model.BrowserEnvStatusError {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_revalidate", envID, "", "validate_status_failed", taskModel.StatusFailed, "revalidate failed", "只有 status=error 的环境包需要 revalidate", "set env to error before revalidate"))
		return
	}
	if _, err = validateAtomicPackage(index); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_revalidate", envID, "", "validate_atomic_materials_failed", taskModel.StatusFailed, "revalidate failed", err.Error(), suggestionForError(err)))
		return
	}
	now := time.Now().Unix()
	if err = browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
		EnvID:           envID,
		Status:          model.BrowserEnvStatusCreated,
		ContainerID:     nil,
		ContainerName:   index.ContainerName,
		ContainerStatus: model.ContainerStatusMissing,
		MonitorStatus:   model.MonitorStatusUnknown,
		LastError:       nil,
		UpdatedAt:       now,
		LastCheckedAt:   &now,
	}); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_revalidate", envID, "", "update_index_failed", taskModel.StatusFailed, "revalidate failed", err.Error(), suggestionForError(err)))
		return
	}
	_ = publisher.PublishCompleted(taskID, newTaskEvent(taskModel.EventCompleted, taskID, "browser_env_revalidate", envID, "", "finalize_success", taskModel.StatusSuccess, "browser env revalidated", "", ""))
}

func (s *Service) executeImport(taskID string, tempArchivePath string) {
	publisher := taskService.GetService()
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_import_package", "", "", "extract_to_staging", taskModel.StatusQueued, "task accepted", "", ""))
	stagingRoot, err := os.MkdirTemp("", "private-browser-import-*")
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", "", "", "create_staging_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	defer os.RemoveAll(stagingRoot)
	defer os.Remove(tempArchivePath)
	file, err := os.Open(tempArchivePath)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", "", "", "open_import_file_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	defer file.Close()
	if err = extractImportTarGz(file, stagingRoot); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", "", "", "extract_import_package_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	root, err := findImportPackageRoot(stagingRoot)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", "", "", "validate_archive_structure_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	var profile model.ProfileFile
	if err = readJSONFile(filepath.Join(root, "profile.json"), &profile); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", "", "", "load_profile_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	envID := profile.EnvID
	if strings.TrimSpace(envID) == "" {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", "", "", "validate_profile_failed", taskModel.StatusFailed, "import failed", "profile.envId 不能为空", "check package profile.json"))
		return
	}
	_ = publisher.PublishProgress(taskID, newTaskEvent(taskModel.EventProgress, taskID, "browser_env_import_package", envID, "", "prepare_import_package", taskModel.StatusRunning, "preparing browser env import", "", ""))
	envSequence, ports, err := nextAvailableEnvSequenceAndPorts()
	if err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", envID, "", "allocate_ports_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	profile.EnvSequence = envSequence
	profile.Ports = ports
	profile.Metadata.UpdatedAt = time.Now().Unix()
	targetRel := filepath.ToSlash(filepath.Join("data", "browser-envs", "users", profile.UserID, profile.RPAType, profile.EnvID))
	targetAbs := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(targetRel))
	if err = ensureEnvPathAvailable(targetAbs); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", envID, "", "check_env_path_conflict_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	if err = copyDirectory(root, targetAbs); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", envID, "", "promote_staging_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	if err = writePackageJSON(targetAbs, "profile.json", profile); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", envID, "", "rewrite_profile_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	now := time.Now().Unix()
	containerName := edgeBrowserContainerName(profile.EnvID)
	if err = browserEnvDao.NewCreateModelHandler().CreateBrowserEnvIndex(&model.BrowserEnvIndex{
		EnvID:           profile.EnvID,
		UserID:          profile.UserID,
		RPAType:         profile.RPAType,
		Name:            profile.Name,
		EnvSequence:     envSequence,
		CDPPort:         ports.CDP,
		VNCPort:         ports.VNC,
		EnvPath:         targetRel,
		Status:          model.BrowserEnvStatusCreated,
		ContainerName:   &containerName,
		ContainerStatus: model.ContainerStatusMissing,
		MonitorStatus:   model.MonitorStatusUnknown,
		HasBrowserData:  true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		_ = publisher.PublishFailed(taskID, newTaskEvent(taskModel.EventFailed, taskID, "browser_env_import_package", envID, "", "create_index_failed", taskModel.StatusFailed, "import failed", err.Error(), suggestionForError(err)))
		return
	}
	_, _ = packageService.NewService().EnsureRuntimeView(profile.EnvID)
	_ = publisher.PublishCompleted(taskID, newTaskEvent(taskModel.EventCompleted, taskID, "browser_env_import_package", envID, "", "finalize_success", taskModel.StatusSuccess, "browser env imported", "", ""))
}

func newTaskEvent(event string, taskID string, taskType string, envID string, slotID string, stage string, status string, message string, errMsg string, suggestion string) taskModel.Event {
	return taskModel.Event{
		Event:        event,
		TaskID:       taskID,
		TaskType:     taskType,
		ResourceType: "browser_env",
		ResourceID:   envID,
		EnvID:        envID,
		SlotID:       slotID,
		Stage:        stage,
		Status:       status,
		Message:      message,
		Error:        errMsg,
		Suggestion:   suggestion,
		Timestamp:    time.Now().Format(time.RFC3339),
	}
}

func suggestionForError(err error) string {
	switch {
	case errors.Is(err, common.ErrConflict):
		return "wait for the current lifecycle action to finish, then retry"
	case errors.Is(err, common.ErrNotFound):
		return "verify envId and slot state on this client"
	default:
		return "check client logs and local runtime state"
	}
}

func derefTime(value *int64) int64 {
	if value == nil {
		return time.Now().Unix()
	}
	return *value
}

func safeString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// acceptedTask 统一生成正式长任务接口的 accepted 响应。
//
// 这里单独抽函数，是为了让 backup/restore/revalidate/import-package/delete
// 等正式接口始终复用同一套 taskId/eventsUrl 输出结构，避免后续协议扩展时多处漏改。
func acceptedTask(taskID string, taskType string, envID string) *model.TaskAcceptedResponse {
	return &model.TaskAcceptedResponse{
		TaskID:       taskID,
		TaskType:     taskType,
		ResourceType: "browser_env",
		ResourceID:   envID,
		EventsURL:    fmt.Sprintf("/api/v1/edge/tasks/%s/events", taskID),
	}
}
