package BrowserEnv

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

// lifecyclePackage 是 stop/status-refresh 这类轻量生命周期动作需要的环境包视图。
//
// 设计来源：
// - run 需要读取 profile、binding、proxy、fingerprint 并做 identityHash 强校验；
// - stop 只需要确认 manifest/container 的运行态事实，不能因为代理文件或指纹 runtime-config 缺失而拒绝停止；
// - 所以这里拆出更小的读取结构，让生命周期动作保持低风险、低侵入。
type lifecyclePackage struct {
	Index           *model.BrowserEnvIndex
	Manifest        model.ManifestFile
	Container       model.ContainerFile
	AbsoluteEnvPath string
}

// StopBrowserEnv 停止环境包对应的本机 Docker 浏览器容器。
//
// 设计来源：
// - 用户确认 BrowserEnv 应进入正规生命周期管理，前端以后只围绕 envId 操作；
// - 直接调用 /edge/containers/:id/stop 虽然能停容器，但不会回写 SQLite 和环境包文件；
// - 因此 stop(envId) 必须成为上层编排入口：查索引、停 Docker、写 container.json、写 manifest、写 browser_envs。
//
// 职责边界：
// - 负责本机环境包生命周期停止和状态同步；
// - 不删除容器、不删除镜像、不删除 browser-data/profile 登录态目录；
// - 不重新计算指纹、不检查代理出口、不做中心服务端上报。
func (s *Service) StopBrowserEnv(envID string, param *model.StopBrowserEnvRequest) (*model.StopBrowserEnvResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}
	if param == nil {
		param = &model.StopBrowserEnvRequest{}
	}

	runEnvMu.Lock()
	defer runEnvMu.Unlock()

	handler := browserEnvDao.NewRuntimeModelHandler()
	index, err := handler.GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}
	if index.Status == model.BrowserEnvStatusDeleted {
		return nil, conflictError("环境包已删除，不能停止")
	}

	pkg, err := loadLifecyclePackage(index)
	if err != nil {
		return nil, internalError(err.Error())
	}
	containerID, containerName := resolveContainerIdentity(index, pkg.Container)
	if strings.TrimSpace(containerID) == "" && strings.TrimSpace(containerName) == "" {
		return buildNoContainerStopResponse(index), nil
	}
	if index.Status != model.BrowserEnvStatusRunning && strings.TrimSpace(containerID) == "" {
		return buildNoContainerStopResponse(index), nil
	}

	action, err := edgeService.NewEdgeService().StopDockerContainer(firstNonEmpty(containerID, containerName), &edgeModel.ContainerActionRequest{
		TimeoutSeconds: param.TimeoutSeconds,
	})
	if err != nil {
		// Docker 404 通常表示数据库或 container.json 里保留了旧容器标识，但容器已被人工删除。
		// 这种情况下环境包事实应收敛为 stopped，而不是让前端一直看到 running。
		if isDockerContainerNotFound(err) {
			return finalizeStopPackage(pkg, "not-found", "container not found, marked as stopped")
		}
		return nil, remoteError(err.Error())
	}
	return finalizeStopPackage(pkg, action.Status, action.Message)
}

// loadLifecyclePackage 读取生命周期动作需要的最小文件集合。
//
// 这里只读 manifest.json 和 container.json；它不会创建目录、不会校验 identityHash，
// 也不会触碰 browser-data/profile，确保 stop 这类动作不会改变登录态持久化载体。
func loadLifecyclePackage(index *model.BrowserEnvIndex) (*lifecyclePackage, error) {
	if index == nil {
		return nil, fmt.Errorf("环境包索引不能为空")
	}
	if Settings.Conf.ProjectRoot == "" {
		return nil, fmt.Errorf("project root 不能为空")
	}
	absoluteEnvPath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(index.EnvPath))
	manifestPath := filepath.Join(absoluteEnvPath, "manifest.json")
	var manifest model.ManifestFile
	if err := readJSONFile(manifestPath, &manifest); err != nil {
		return nil, err
	}
	if manifest.EnvID != index.EnvID {
		return nil, fmt.Errorf("manifest.envId 与数据库索引不一致")
	}
	var container model.ContainerFile
	if err := readPackageJSON(absoluteEnvPath, manifest.Paths.Container, &container); err != nil {
		return nil, err
	}
	return &lifecyclePackage{
		Index:           index,
		Manifest:        manifest,
		Container:       container,
		AbsoluteEnvPath: absoluteEnvPath,
	}, nil
}

// resolveContainerIdentity 从数据库索引和 container.json 中解析容器标识。
//
// 优先使用 containerId，因为 Docker ID 最稳定；如果早期数据没有 containerId，
// 再退回 containerName，兼容按名称停止容器。
func resolveContainerIdentity(index *model.BrowserEnvIndex, container model.ContainerFile) (string, string) {
	containerID := ""
	if index != nil && index.ContainerID != nil {
		containerID = strings.TrimSpace(*index.ContainerID)
	}
	if containerID == "" && container.ContainerID != nil {
		containerID = strings.TrimSpace(*container.ContainerID)
	}

	containerName := ""
	if index != nil && index.ContainerName != nil {
		containerName = strings.TrimSpace(*index.ContainerName)
	}
	if containerName == "" {
		containerName = strings.TrimSpace(container.ContainerName)
	}
	return containerID, containerName
}

// finalizeStopPackage 在 Docker 停止结果明确后回写环境包文件和数据库。
//
// 写入范围只限运行态字段：status、stoppedAt、lastRuntime.lastStoppedAt、last_checked_at。
// 端口、镜像、代理、指纹、browser-data 路径都不在 stop 阶段修改，避免停止动作破坏环境身份。
func finalizeStopPackage(pkg *lifecyclePackage, actionStatus string, message string) (*model.StopBrowserEnvResponse, error) {
	now := time.Now().Unix()
	containerID, containerName := resolveContainerIdentity(pkg.Index, pkg.Container)

	if strings.TrimSpace(containerID) != "" {
		pkg.Container.ContainerID = &containerID
	}
	if strings.TrimSpace(containerName) != "" {
		pkg.Container.ContainerName = containerName
	}
	pkg.Container.Status = model.BrowserEnvStatusStopped
	pkg.Container.StoppedAt = &now
	pkg.Container.UpdatedAt = now

	pkg.Manifest.LastRuntime.ContainerID = optionalString(containerID)
	pkg.Manifest.LastRuntime.ContainerName = optionalString(containerName)
	pkg.Manifest.LastRuntime.LastStoppedAt = &now
	pkg.Manifest.UpdatedAt = now

	if err := writeJSONFile(filepath.Join(pkg.AbsoluteEnvPath, filepath.FromSlash(pkg.Manifest.Paths.Container)), pkg.Container); err != nil {
		return nil, internalError(err.Error())
	}
	if err := writeJSONFile(filepath.Join(pkg.AbsoluteEnvPath, "manifest.json"), pkg.Manifest); err != nil {
		return nil, internalError(err.Error())
	}

	if err := browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(context.Background(), &model.BrowserEnvRuntimeUpdate{
		EnvID:           pkg.Index.EnvID,
		Status:          model.BrowserEnvStatusStopped,
		ContainerID:     optionalString(containerID),
		ContainerName:   optionalString(containerName),
		ContainerStatus: model.BrowserEnvStatusStopped,
		MonitorStatus:   model.BrowserEnvMonitorStatusUnknown,
		UpdatedAt:       now,
		LastStartedAt:   pkg.Index.LastStartedAt,
		LastStoppedAt:   &now,
		LastCheckedAt:   &now,
	}); err != nil {
		return nil, internalError(err.Error())
	}

	return &model.StopBrowserEnvResponse{
		EnvID:           pkg.Index.EnvID,
		ContainerID:     optionalString(containerID),
		ContainerName:   optionalString(containerName),
		Status:          model.BrowserEnvStatusStopped,
		ContainerStatus: model.BrowserEnvStatusStopped,
		ActionStatus:    actionStatus,
		Message:         message,
		StoppedAt:       now,
	}, nil
}

// buildNoContainerStopResponse 处理“环境包没有容器事实”的幂等停止场景。
//
// created 状态的环境包还没 run 过，可能只有配置文件而没有 containerId；
// 这种情况下 stop 不应该访问 Docker，也不应该把未启动环境强行改成 stopped。
func buildNoContainerStopResponse(index *model.BrowserEnvIndex) *model.StopBrowserEnvResponse {
	now := time.Now().Unix()
	envID := ""
	status := ""
	containerStatus := model.BrowserEnvContainerStatusUnknown
	var containerName *string
	if index != nil {
		envID = index.EnvID
		status = index.Status
		containerStatus = index.ContainerStatus
		containerName = index.ContainerName
	}
	return &model.StopBrowserEnvResponse{
		EnvID:           envID,
		ContainerName:   containerName,
		Status:          status,
		ContainerStatus: containerStatus,
		ActionStatus:    "not-modified",
		Message:         "container not created",
		StoppedAt:       now,
	}
}

// optionalString 把空字符串转换成 nil 指针。
//
// 数据库和 JSON 中的 containerId/containerName 都是可选运行态字段；
// 空字符串不应写成有效值，否则前端和后续状态刷新会误认为已有可操作容器。
func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// firstNonEmpty 选择 Docker 停止动作使用的容器标识。
//
// Docker Engine API 既接受 containerId 也接受 containerName；
// 上层已经按“ID 优先、名称兜底”排好顺序，这里只做空值过滤，避免把空字符串传给 Docker。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// isDockerContainerNotFound 识别 Docker 404 的旧容器记录场景。
//
// 这个判断只用于 stop(envId)：容器被人工删除时，环境包应该收敛为 stopped；
// 其他 Docker 错误仍然作为远端错误返回，不能假装停止成功。
func isDockerContainerNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "status=404")
}
