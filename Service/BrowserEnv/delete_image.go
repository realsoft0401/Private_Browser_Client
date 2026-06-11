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
)

// DeleteBrowserEnvImage 删除环境包关联的 Docker 镜像。
//
// 设计来源：
// - 用户要求把镜像删除和环境包销毁拆成两个独立端点：/del 只删镜像，/package 才销毁环境包；
// - 当前是重新开发阶段，不保留 /delimage 或根 DELETE 这类冗余删除入口，避免权限边界被旧路径稀释；
// - 镜像删除只影响本机 Docker，不碰环境包目录、browser-data/profile 和 SQLite 索引；
// - 如果镜像还被其他环境包的容器引用，Docker 会因为冲突层保护拒绝删除，Service 只如实返回结果。
//
// 职责边界：
// - 负责从环境包 profile.json 读取 runtime.image，调用 Docker API 删除镜像；
// - 不停止运行中容器、不删除容器、不修改环境包文件、不写 SQLite；
// - 如果 envId 对应的环境包仍 running，直接拒绝，避免删除正在使用的镜像。
func (s *Service) DeleteBrowserEnvImage(envID string) (*model.DeleteBrowserEnvImageResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}

	index, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}

	// 运行中的环境包拒绝删除镜像，避免正在被容器使用的镜像被删。
	if index.Status == model.BrowserEnvStatusRunning || index.ContainerStatus == model.BrowserEnvStatusRunning {
		return nil, conflictError("环境包正在运行，无法删除正在使用的镜像；请先 stop")
	}

	runtimeImage, err := readRuntimeImageFromProfile(index)
	if err != nil {
		return nil, internalError(err.Error())
	}
	if runtimeImage == "" {
		return nil, invalidError("环境包 profile.runtime.image 为空，没有镜像可以删除")
	}

	results, err := edgeService.NewEdgeService().RemoveDockerImage(&edgeModel.RemoveImageRequest{
		Image: runtimeImage,
	})
	now := time.Now().Unix()

	if err != nil {
		// 检查是否镜像还被其他容器使用。
		// Docker 会在镜像仍被容器引用时拒绝删除，返回 "image is being used by running container" 或
		// "unable to delete" 等明确错误。这里只匹配 Docker 镜像删除的特有错误模式，
		// 不使用宽泛的 "conflict" 匹配，避免把端口冲突、名称冲突等无关错误误判为镜像引用问题。
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "image is being used") ||
			strings.Contains(errStr, "image has dependent") ||
			strings.Contains(errStr, "unable to delete") {
			return &model.DeleteBrowserEnvImageResponse{
				EnvID:          envID,
				Image:          runtimeImage,
				ImageRemoved:   false,
				DeletedAt:      now,
				WarningMessage: "镜像删除被 Docker 拒绝，可能仍被其他容器引用。请先停止并删除所有使用该镜像的容器后再重试。",
			}, nil
		}
		return nil, internalError(fmt.Sprintf("删除 Docker 镜像失败: %v", err))
	}

	resp := &model.DeleteBrowserEnvImageResponse{
		EnvID:     envID,
		Image:     runtimeImage,
		Results:   make([]model.DockerImageRemoveResultRef, 0, len(results)),
		DeletedAt: now,
	}

	for _, r := range results {
		resp.Results = append(resp.Results, model.DockerImageRemoveResultRef{
			Image:    runtimeImage,
			Deleted:  r.Deleted,
			Untagged: r.Untagged,
		})
	}
	if len(resp.Results) == 0 {
		// Docker 返回空结果可能是镜像已经不存在。
		resp.ImageRemoved = true
		resp.WarningMessage = "Docker 未返回删除结果；镜像可能已不存在"
	} else {
		resp.ImageRemoved = true
	}

	return resp, nil
}

// readRuntimeImageFromProfile 从环境包 profile.json 读取 runtime.image。
//
// 设计来源：
// - 环境包的镜像引用只来自 profile.json 的 runtime.image，不从 SQLite 或 container.json 猜测；
// - 这里只做最小读取，不校验整个环境包原子性，避免轻量镜像删除操作被过度阻塞。
func readRuntimeImageFromProfile(index *model.BrowserEnvIndex) (string, error) {
	if index == nil {
		return "", fmt.Errorf("环境包索引不能为空")
	}

	// backed_up 状态的环境包运行目录已被释放，profile.json 不存在。
	// 此时无法读取 runtime.image，应提示用户直接使用 /api/v1/edge/docker/remove-image。
	if index.Status == model.BrowserEnvStatusBackedUp || index.Status == model.BrowserEnvStatusArchived {
		return "", fmt.Errorf(
			"环境包状态为 %s，运行目录已释放，无法读取 profile.json 中的镜像引用。"+
				"如需删除镜像，请直接使用 /api/v1/edge/docker/remove-image 并指定镜像名",
			index.Status,
		)
	}

	absPath, err := resolveManagedEnvPath(index)
	if err != nil {
		return "", fmt.Errorf("解析环境包路径失败: %w", err)
	}

	var profile model.ProfileFile
	profilePath := filepath.Join(absPath, "profile.json")
	if err := readJSONFile(profilePath, &profile); err != nil {
		return "", fmt.Errorf("读取 profile.json 失败 (%s): %w", profilePath, err)
	}
	runtimeImage := strings.TrimSpace(profile.Runtime.Image)
	return runtimeImage, nil
}
