package Edge

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	model "private_browser_client/Models/Edge"
	"private_browser_client/Settings"
)

// GetDockerStatus 返回当前 Client 本机 Docker 的最小健康摘要。
//
// 设计来源：
// - 这条接口是后续所有 Docker 排障动作之前的第一条只读检查；
// - 前端和 Node 都需要快速知道“本机 Docker 能不能用”，而不是一上来就拉全量镜像/容器列表；
// - 当前项目已经明确 Client 只负责本机 Docker，因此这里不混入任何中心字段。
//
// 职责边界：
// - 只探测本机 Docker API；
// - 只返回可用性、镜像数、容器数；
// - 不写 SQLite，不创建 task。
func (s *Service) GetDockerStatus() (*model.DockerStatus, error) {
	dockerAPIURL := currentDockerAPIURL()
	if dockerAPIURL == "" {
		return nil, fmt.Errorf("docker api url 不能为空")
	}

	if _, err := s.fetchRaw(dockerAPIURL, http.MethodGet, "/_ping", nil); err != nil {
		return nil, fmt.Errorf("docker api ping failed: %w", err)
	}

	info := new(model.DockerEngineInfoResponse)
	if err := s.fetchJSON(dockerAPIURL, http.MethodGet, "/info", nil, info); err != nil {
		return nil, fmt.Errorf("docker api info failed: %w", err)
	}

	return &model.DockerStatus{
		DockerAPIURL:    dockerAPIURL,
		Status:          "available",
		Message:         "docker is reachable",
		ImagesCount:     info.Images,
		ContainersCount: info.Containers,
		CheckedAt:       time.Now().Unix(),
	}, nil
}

// GetDockerImages 返回当前 Client 本机的镜像摘要列表。
//
// 职责边界：
// - 只读；
// - 不做镜像策略判断；
// - 不因为镜像存在就承诺 browser-env 一定可以运行。
func (s *Service) GetDockerImages() ([]model.DockerImage, error) {
	dockerAPIURL := currentDockerAPIURL()
	if dockerAPIURL == "" {
		return nil, fmt.Errorf("docker api url 不能为空")
	}

	rawList := make([]model.DockerEngineImageResponse, 0)
	if err := s.fetchJSON(dockerAPIURL, http.MethodGet, "/images/json", nil, &rawList); err != nil {
		return nil, fmt.Errorf("docker api images failed: %w", err)
	}

	images := make([]model.DockerImage, 0, len(rawList))
	for _, raw := range rawList {
		images = append(images, model.DockerImage{
			ID:          strings.TrimSpace(raw.ID),
			RepoTags:    normalizeStringSlice(raw.RepoTags),
			RepoDigests: normalizeStringSlice(raw.RepoDigests),
			Created:     raw.Created,
			Size:        raw.Size,
		})
	}
	return images, nil
}

// GetDockerContainers 返回当前项目相关的本机容器摘要列表。
//
// 设计来源：
// - 用户已经明确不要把宿主机所有无关容器暴露出来；
// - 当前新 Client 只需要关心本项目创建的 slot 容器和 browser-env 容器；
// - 识别口径优先看 `bv.project/bv.role`，再兼容历史命名前缀。
func (s *Service) GetDockerContainers() ([]model.DockerContainer, error) {
	dockerAPIURL := currentDockerAPIURL()
	if dockerAPIURL == "" {
		return nil, fmt.Errorf("docker api url 不能为空")
	}

	rawList := make([]model.DockerEngineContainerResponse, 0)
	if err := s.fetchJSON(dockerAPIURL, http.MethodGet, "/containers/json?all=true", nil, &rawList); err != nil {
		return nil, fmt.Errorf("docker api containers failed: %w", err)
	}

	containers := make([]model.DockerContainer, 0, len(rawList))
	for _, raw := range rawList {
		container, ok := mapProjectContainer(raw)
		if !ok {
			continue
		}
		containers = append(containers, container)
	}
	return containers, nil
}

func mapProjectContainer(raw model.DockerEngineContainerResponse) (model.DockerContainer, bool) {
	labels := normalizeStringMap(raw.Labels)
	names := normalizeContainerNames(raw.Names)

	role := ""
	switch {
	case labels["bv.project"] == "private-browser-client" && labels["bv.role"] != "":
		role = labels["bv.role"]
	case hasSlotContainerName(names):
		role = "slot-runtime"
	default:
		return model.DockerContainer{}, false
	}

	return model.DockerContainer{
		ID:          strings.TrimSpace(raw.ID),
		Names:       names,
		Image:       strings.TrimSpace(raw.Image),
		State:       strings.TrimSpace(raw.State),
		Status:      strings.TrimSpace(raw.Status),
		ProjectRole: role,
		SlotID:      strings.TrimSpace(labels["bv.slotId"]),
		EnvID:       strings.TrimSpace(labels["bv.envId"]),
	}, true
}

func hasSlotContainerName(names []string) bool {
	prefix := "private-browser-slot-"
	if Settings.Conf != nil && Settings.Conf.SlotRuntimeConfig != nil && strings.TrimSpace(Settings.Conf.SlotRuntimeConfig.ContainerNamePrefix) != "" {
		prefix = strings.TrimSpace(Settings.Conf.SlotRuntimeConfig.ContainerNamePrefix) + "-"
	}
	for _, name := range names {
		if strings.HasPrefix(strings.TrimSpace(name), prefix) {
			return true
		}
	}
	return false
}

func normalizeContainerNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	result := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(strings.TrimPrefix(name, "/"))
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}
