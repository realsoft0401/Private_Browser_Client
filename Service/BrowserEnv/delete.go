package BrowserEnv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

// DeleteBrowserEnv 对本机环境包执行受保护的物理删除。
//
// 设计来源：
// - 用户确认 DELETE 应代表用户不再需要这个环境包，配置文件和 browser-data/profile 都应彻底删除；
// - 如果只标记 status=deleted，后续 rebuild-index 兜底扫描目录时可能把用户明确删除的包重新恢复出来；
// - 因此前端负责强提示“删除后无法找回”，后端负责路径安全校验、删除目录和移除 SQLite 索引。
//
// 职责边界：
// - 删除范围只允许落在 data/browser-envs 下，且目录名必须等于 envId，避免异常 env_path 误删项目目录；
// - 不自动停止容器、不删除镜像，运行中的环境必须先走 stop(envId)；
// - 如果索引里还有已停止容器标识，会删除这个容器，避免留下挂载目录已丢失的 Docker 垃圾容器；
// - 目录删除成功后移除 browser_envs 索引，保证 rebuild-index 不会恢复这个环境包。
func (s *Service) DeleteBrowserEnv(envID string) (*model.DeleteBrowserEnvResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}

	runEnvMu.Lock()
	defer runEnvMu.Unlock()

	handler := browserEnvDao.NewDeleteModelHandler()
	index, err := handler.GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}

	if index.Status == model.BrowserEnvStatusRunning || index.ContainerStatus == model.BrowserEnvStatusRunning {
		return nil, conflictError("环境包正在运行，请先停止后再删除")
	}

	if err = removeStoppedContainerForDelete(index); err != nil {
		return nil, err
	}

	absoluteEnvPath, err := resolveManagedEnvPath(index)
	if err != nil {
		return nil, internalError(err.Error())
	}
	if err = os.RemoveAll(absoluteEnvPath); err != nil {
		return nil, internalError(fmt.Sprintf("删除环境包目录失败: %v", err))
	}
	if err = handler.DeleteBrowserEnvIndex(context.Background(), index.EnvID); err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}

	now := time.Now().Unix()
	return &model.DeleteBrowserEnvResponse{
		EnvID:        index.EnvID,
		Status:       model.BrowserEnvStatusDeleted,
		ActionStatus: "deleted",
		Message:      "环境包已彻底删除，配置文件、browser-data/profile 和关联容器无法找回",
		DeletedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// removeStoppedContainerForDelete 删除环境包关联的已停止 Docker 容器。
//
// 设计来源：
// - 删除环境包目录后，旧容器的 bind mount 会指向一个已经不存在的 browser-data/profile；
// - 保留这种容器既无法正常恢复环境，也会污染本项目容器列表；
// - 因此在确认环境包非 running 后，删除索引中记录的容器，但不删除镜像、不自动停止运行中容器。
func removeStoppedContainerForDelete(index *model.BrowserEnvIndex) error {
	if index == nil {
		return internalError("环境包索引不能为空")
	}
	containerID := ""
	if index.ContainerID != nil {
		containerID = strings.TrimSpace(*index.ContainerID)
	}
	containerName := ""
	if index.ContainerName != nil {
		containerName = strings.TrimSpace(*index.ContainerName)
	}
	containerKey := firstNonEmpty(containerID, containerName)
	if containerKey == "" {
		return nil
	}
	if err := edgeService.NewEdgeService().RemoveDockerContainer(containerKey, false); err != nil {
		if isDockerContainerNotFound(err) {
			return nil
		}
		return remoteError(err.Error())
	}
	return nil
}

// resolveManagedEnvPath 校验并返回允许操作的环境包绝对路径。
//
// 删除和导出打包都不能直接信任数据库里的 env_path：
// - env_path 必须是相对路径；
// - 解析后的路径必须位于 data/browser-envs 子树内；
// - 最后一层目录名必须等于 envId，避免误删或误打包 users/{userId}/{rpaType} 这类上级目录。
func resolveManagedEnvPath(index *model.BrowserEnvIndex) (string, error) {
	if index == nil {
		return "", fmt.Errorf("环境包索引不能为空")
	}
	if Settings.Conf.ProjectRoot == "" {
		return "", fmt.Errorf("project root 不能为空")
	}
	envPath := strings.TrimSpace(index.EnvPath)
	if envPath == "" {
		return "", fmt.Errorf("envPath 不能为空")
	}
	if filepath.IsAbs(envPath) {
		return "", fmt.Errorf("envPath 必须是项目内相对路径")
	}

	baseAbs, err := filepath.Abs(filepath.Join(Settings.Conf.ProjectRoot, "data", "browser-envs"))
	if err != nil {
		return "", fmt.Errorf("解析 browser-envs 根目录失败: %w", err)
	}
	targetAbs, err := filepath.Abs(filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(envPath)))
	if err != nil {
		return "", fmt.Errorf("解析环境包目录失败: %w", err)
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("校验环境包目录失败: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("envPath 不在 data/browser-envs 目录内")
	}
	if filepath.Base(targetAbs) != index.EnvID {
		return "", fmt.Errorf("envPath 最后一层目录必须等于 envId")
	}
	return targetAbs, nil
}
