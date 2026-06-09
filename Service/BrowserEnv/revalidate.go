package BrowserEnv

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

// RevalidateBrowserEnv 对 error 环境执行受控重新准入。
//
// 设计来源：
// - 用户确认网络指纹、代理和 browser-data/profile 是账号环境原子整体；
// - run 探测失败后即使容器还在 running，也不能让普通 run/stop/proxy update 把 error 悄悄覆盖；
// - 管理员先通过裸容器接口或宿主机排查，再调用 revalidate，Client 只重新校验可从环境包推导出的事实。
//
// 职责边界：
// - 校验 profile/binding/proxy/fingerprint/browser-data 原子材料；
// - 检查 Docker 上是否存在身份冲突，并根据端口占用重新分配本机端口；
// - 把环境恢复到 created 或 stopped，且 runtimeProtection/proxyRuntime 只能标记 pending；
// - 不启动容器、不拉镜像、不创建容器、不证明代理出口或 timezone 最终可用。
func (s *Service) RevalidateBrowserEnv(envID string) (*model.RevalidateBrowserEnvResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
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
		return nil, conflictError("环境包已删除，不能 revalidate")
	}
	if index.Status == model.BrowserEnvStatusBackedUp || index.Status == model.BrowserEnvStatusArchived {
		return nil, conflictError("环境包当前只有备份包，请先 restore 后再 revalidate")
	}
	if index.Status != model.BrowserEnvStatusError {
		return nil, conflictError("只有 status=error 的环境包需要 revalidate；当前状态不能通过 revalidate 改写")
	}

	envPath, err := resolveManagedEnvPath(index)
	if err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, internalError(err.Error())
	}
	atomic, err := loadAndValidateAtomicPackage(envPath)
	if err != nil {
		_ = updateRunError(envID, err.Error())
		if bizErr, ok := IsBusinessError(err); ok {
			return nil, conflictError("revalidate 失败，环境包原子材料不完整或不可信: " + bizErr.Message)
		}
		return nil, internalError(err.Error())
	}

	container := atomic.Container
	if !atomic.HasContainerFile {
		container = model.ContainerFile{EnvID: atomic.Profile.EnvID, Image: atomic.Profile.Runtime.Image}
	}
	if strings.TrimSpace(container.ContainerName) == "" {
		container.ContainerName = "bv-" + strings.ReplaceAll(atomic.Profile.EnvID, "_", "-")
	}

	edge := edgeService.NewEdgeService()
	containers, err := edge.GetDockerContainers()
	if err != nil {
		message := "Docker API 不可达，不能证明异常环境是否存在容器身份冲突；请检查 Docker 2375、本机网络和服务权限后重试: " + err.Error()
		_ = updateRunError(envID, message)
		return nil, remoteError(message)
	}
	match, err := resolveRevalidateDockerFact(index, atomic.Profile.EnvID, container.ContainerName, containers)
	if err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, conflictError(err.Error())
	}

	envSequence, ports, err := restoreRuntimePorts(index)
	if err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, conflictError("revalidate 失败，无法分配可用 CDP/VNC 端口: " + err.Error())
	}

	now := time.Now().Unix()
	targetStatus, containerStatus, containerID, containerName, lastStoppedAt, message, err := revalidateTargetFromDocker(index, container.ContainerName, match, now)
	if err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, conflictError(err.Error())
	}

	profile := atomic.Profile
	binding := atomic.Binding
	resetRevalidateFiles(&profile, &binding, &container, envSequence, ports, targetStatus, containerID, containerName, lastStoppedAt, now)
	if err = writePackageJSON(envPath, profile.Paths.Binding, binding); err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, internalError(err.Error())
	}
	if err = writeTimezoneProbePending(envPath, profile.Paths.ProxyRuntime); err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(envPath, profile.Paths.Container, container); err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(envPath, profile.Paths.Profile, profile); err != nil {
		_ = updateRunError(envID, err.Error())
		return nil, internalError(err.Error())
	}

	if err = handler.UpdateBrowserEnvRuntime(context.Background(), &model.BrowserEnvRuntimeUpdate{
		EnvID:           index.EnvID,
		Status:          targetStatus,
		EnvSequence:     &envSequence,
		CDPPort:         &ports.CDP,
		VNCPort:         &ports.VNC,
		ContainerID:     containerID,
		ContainerName:   containerName,
		ContainerStatus: containerStatus,
		MonitorStatus:   model.BrowserEnvMonitorStatusUnknown,
		LastError:       nil,
		UpdatedAt:       now,
		LastStartedAt:   nil,
		LastStoppedAt:   lastStoppedAt,
		LastCheckedAt:   &now,
	}); err != nil {
		return nil, internalError(err.Error())
	}

	return &model.RevalidateBrowserEnvResponse{
		EnvID:                   index.EnvID,
		Status:                  targetStatus,
		ContainerStatus:         containerStatus,
		ContainerID:             containerID,
		ContainerName:           containerName,
		EnvSequence:             envSequence,
		Ports:                   ports,
		RuntimeProtectionStatus: "pending",
		RevalidatedAt:           now,
		Message:                 message,
	}, nil
}

// resolveRevalidateDockerFact 从本机 Docker 列表中确认异常环境是否有唯一、可信的容器事实。
//
// 它只做身份判定，不停止、不删除、不启动容器。若发现同 envId 多容器、同名异 envId、
// 或 SQLite/container.json 指向了其它环境的容器，必须失败交给管理员排查，避免把账号环境绑定到错容器。
func resolveRevalidateDockerFact(index *model.BrowserEnvIndex, envID string, containerName string, containers []edgeModel.DockerContainer) (*edgeModel.DockerContainer, error) {
	var own []edgeModel.DockerContainer
	for _, container := range containers {
		nameMatched := dockerContainerHasName(container, containerName)
		idMatched := index != nil && index.ContainerID != nil && strings.TrimSpace(*index.ContainerID) != "" && strings.TrimSpace(container.ID) == strings.TrimSpace(*index.ContainerID)
		envMatched := strings.TrimSpace(container.EnvID) == envID || strings.TrimSpace(container.Labels["bv.envId"]) == envID
		if nameMatched && !envMatched {
			return nil, fmt.Errorf("Docker 中存在同名容器 %s 但 envId 不匹配，不能 revalidate；请管理员先处理容器身份冲突", containerName)
		}
		if idMatched && !envMatched {
			return nil, fmt.Errorf("SQLite 记录的 containerId 指向了其它环境，不能 revalidate；请管理员先处理 Docker 身份冲突")
		}
		if envMatched || nameMatched || idMatched {
			own = append(own, container)
		}
	}
	if len(own) > 1 {
		return nil, fmt.Errorf("Docker 中找到多个匹配 envId=%s 的容器，不能自动选择；请管理员先清理重复容器", envID)
	}
	if len(own) == 0 {
		return nil, nil
	}
	return &own[0], nil
}

// revalidateTargetFromDocker 把唯一 Docker 事实映射为允许恢复的资产状态。
//
// running/restarting/paused 都不进入正常生命周期：这些状态仍可能保留未验证的网络指纹或浏览器写盘，
// 必须先由管理员用裸容器诊断/停止；missing 则恢复为 created，已退出容器恢复为 stopped。
func revalidateTargetFromDocker(index *model.BrowserEnvIndex, fallbackName string, match *edgeModel.DockerContainer, now int64) (string, string, *string, *string, *int64, string, error) {
	if match == nil {
		return model.BrowserEnvStatusCreated, "missing", nil, optionalString(fallbackName), nil, "revalidate 通过：环境包原子材料完整，未发现可用容器，已恢复为 created；下一步需要显式 run", nil
	}
	state := strings.ToLower(strings.TrimSpace(match.State))
	switch state {
	case "created", "exited":
		name := firstContainerName(*match, fallbackName)
		stoppedAt := index.LastStoppedAt
		if stoppedAt == nil {
			stoppedAt = &now
		}
		return model.BrowserEnvStatusStopped, state, optionalString(match.ID), optionalString(name), stoppedAt, "revalidate 通过：环境包原子材料完整，Docker 容器处于非运行态，已恢复为 stopped；下一步需要显式 run", nil
	case "running", "restarting", "paused":
		return "", state, optionalString(match.ID), optionalString(firstContainerName(*match, fallbackName)), nil, "", fmt.Errorf("Docker 容器仍处于 %s，不能 revalidate 进入正常生命周期；请管理员先用裸容器接口诊断/停止，再重试", state)
	case "dead":
		return "", state, optionalString(match.ID), optionalString(firstContainerName(*match, fallbackName)), nil, "", fmt.Errorf("Docker 容器 state=dead，不能 revalidate；请管理员先删除或修复异常容器")
	default:
		if state == "" {
			state = model.BrowserEnvContainerStatusUnknown
		}
		return "", state, optionalString(match.ID), optionalString(firstContainerName(*match, fallbackName)), nil, "", fmt.Errorf("Docker 容器状态 %s 不能自动判定为可恢复；请管理员排查后重试", state)
	}
}

// resetRevalidateFiles 回写 revalidate 允许修改的运行态字段。
//
// 这里故意不改 envId/userId/rpaType/identityHash、代理明文、fingerprint raw 和 browser-data/profile。
// 这些字段属于账号环境身份或登录态核心资产，revalidate 只能从现有文件证明它们完整，不能重建或替换。
func resetRevalidateFiles(profile *model.ProfileFile, binding *model.BindingFile, container *model.ContainerFile, envSequence int, ports model.BrowserEnvPorts, status string, containerID *string, containerName *string, lastStoppedAt *int64, now int64) {
	profile.EnvSequence = envSequence
	profile.Ports = ports
	profile.LastRuntime.ContainerID = containerID
	profile.LastRuntime.ContainerName = containerName
	profile.LastRuntime.LastStartedAt = nil
	profile.LastRuntime.LastStoppedAt = lastStoppedAt
	profile.LastRuntime.DockerAPIURL = optionalString(Settings.Conf.DockerConfig.APIURL)
	profile.Metadata.UpdatedAt = now

	binding.RuntimeProtection.FingerprintRestored = nil
	binding.RuntimeProtection.RuntimeDrift = nil
	binding.RuntimeProtection.ExitIPChanged = nil
	binding.RuntimeProtection.HighRisk = nil
	binding.RuntimeProtection.LastCheckedAt = nil
	binding.RuntimeProtection.LastVerifiedAt = nil
	binding.RuntimeProtection.TimezoneStatus = "pending"
	binding.RuntimeProtection.RiskStatus = "pending"
	binding.RuntimeProtection.AvailabilityStatus = "pending"
	binding.RuntimeProtection.LastError = ""

	container.EnvID = profile.EnvID
	if containerName != nil && strings.TrimSpace(*containerName) != "" {
		container.ContainerName = strings.TrimSpace(*containerName)
	}
	container.ContainerID = containerID
	container.Image = profile.Runtime.Image
	container.Status = status
	if status == model.BrowserEnvStatusStopped {
		container.Status = model.BrowserEnvStatusStopped
	} else if status == model.BrowserEnvStatusCreated {
		container.Status = model.BrowserEnvStatusCreated
	}
	container.Ports = ports
	container.Docker.APIURL = Settings.Conf.DockerConfig.APIURL
	container.StartedAt = nil
	container.StoppedAt = lastStoppedAt
	container.UpdatedAt = now
	if container.CreatedAt == 0 {
		container.CreatedAt = now
	}
	if container.Labels == nil {
		container.Labels = map[string]string{}
	}
	container.Labels["bv.project"] = "private-browser-client"
	container.Labels["bv.role"] = "browser-env"
	container.Labels["bv.envId"] = profile.EnvID
	container.Labels["bv.userId"] = profile.UserID
	container.Labels["bv.rpaType"] = profile.RPAType
}

// dockerContainerHasName 判断 Docker 容器是否使用指定名称。
//
// Docker API 返回的 Names 通常带有前导 "/"，而 container.json 保存的是业务容器名；
// 这里集中归一化，避免 revalidate 身份判断遗漏同名冲突。
func dockerContainerHasName(container edgeModel.DockerContainer, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, item := range container.Names {
		if strings.TrimPrefix(strings.TrimSpace(item), "/") == name {
			return true
		}
	}
	return false
}
