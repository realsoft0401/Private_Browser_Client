package Edge

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	model "private_browser_client/Models/Edge"
	"private_browser_client/Settings"
)

// CreateDockerContainer 在本机 Docker 上创建一个受控容器。
//
// 设计来源：
// - slot 创建后需要立即准备常驻运行资源；
// - Docker HTTP 细节必须继续收口在 Edge 服务里，不能散落到 Slot Service。
func (s *Service) CreateDockerContainer(name string, config *model.DockerContainerCreateConfig) (*model.DockerContainerCreateResult, error) {
	containerName := strings.TrimSpace(name)
	if containerName == "" {
		return nil, fmt.Errorf("container name 不能为空")
	}
	if config == nil {
		return nil, fmt.Errorf("container config 不能为空")
	}

	bodyBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode docker create config failed: %w", err)
	}

	result := new(model.DockerContainerCreateResult)
	path := "/containers/create?name=" + url.QueryEscape(containerName)
	if err = s.fetchDockerJSON(http.MethodPost, path, bytes.NewReader(bodyBytes), result); err != nil {
		return nil, fmt.Errorf("docker api create container failed: %w", err)
	}
	return result, nil
}

// StartDockerContainer 启动已经创建好的本机容器。
func (s *Service) StartDockerContainer(containerID string) (*model.ContainerActionResult, error) {
	return s.executeContainerAction(containerID, "start", nil)
}

// StopDockerContainer 优雅停止本机 Docker 容器。
//
// 设计来源：
// - 这次 TK 回归已经证明“直接删容器再重建”会导致登录态文件不完整；
// - old 系统 stop 一直先调用 Docker stop，再做状态回写；
// - 因此新 Client 必须恢复这条能力，让 BrowserEnv.stop/backup 能在同一条受控链路上复用。
//
// 职责边界：
// - 这里只负责 Docker stop 动作，不直接改 BrowserEnv、slot、SQLite；
// - timeoutSeconds 只映射 Docker `t` 参数，不引入重试或强行 kill 的隐式行为。
func (s *Service) StopDockerContainer(containerID string, param *model.ContainerActionRequest) (*model.ContainerActionResult, error) {
	timeoutSeconds, err := normalizeContainerTimeout(param, 10)
	if err != nil {
		return nil, err
	}
	return s.executeContainerAction(containerID, "stop", &timeoutSeconds)
}

// RemoveDockerContainer 删除 slot 初始化出的本机容器。
func (s *Service) RemoveDockerContainer(containerID string, force bool) error {
	id := strings.TrimSpace(containerID)
	if id == "" {
		return fmt.Errorf("container id 不能为空")
	}

	path := "/containers/" + url.PathEscape(id)
	if force {
		path += "?force=1"
	}
	if _, err := s.fetchDockerAction(http.MethodDelete, path, nil); err != nil {
		return fmt.Errorf("docker api remove container failed: %w", err)
	}
	return nil
}

// PullDockerImage 拉取本机 Docker 镜像。
//
// 当前只服务 slot 初始化链路的“缺镜像自动拉取”，
// 后续如果恢复正式镜像管理接口，可以继续沿用这条实现。
func (s *Service) PullDockerImage(image string) ([]model.DockerPullEvent, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return nil, fmt.Errorf("image 不能为空")
	}

	query := url.Values{}
	query.Set("fromImage", image)

	var events []model.DockerPullEvent
	if err := s.fetchDockerStream(http.MethodPost, "/images/create?"+query.Encode(), nil, func(line []byte) error {
		event := model.DockerPullEvent{}
		if err := json.Unmarshal(line, &event); err != nil {
			return err
		}
		events = append(events, event)
		if event.Error != "" {
			return fmt.Errorf("%s", event.Error)
		}
		return nil
	}); err != nil {
		return events, fmt.Errorf("docker api pull image failed: %w", err)
	}
	return events, nil
}

func (s *Service) fetchDockerJSON(method string, path string, body io.Reader, target any) error {
	return s.fetchJSON(currentDockerAPIURL(), method, path, body, target)
}

func (s *Service) fetchJSON(baseURL string, method string, path string, body io.Reader, target any) error {
	raw, err := s.fetchRaw(baseURL, method, path, body)
	if err != nil {
		return err
	}
	if target == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err = json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode docker response failed: %w", err)
	}
	return nil
}

func (s *Service) fetchDockerAction(method string, path string, body io.Reader) ([]byte, error) {
	return s.fetchRaw(currentDockerAPIURL(), method, path, body)
}

// executeContainerAction 统一执行 Docker 容器生命周期动作。
//
// 这里延续 old 已验证过的约束：只允许内部固定调用 start/stop，
// 不把任意 action 暴露给上层，避免浏览器运行链路退回黑盒 Docker 透传。
func (s *Service) executeContainerAction(containerID string, action string, timeoutSeconds *int) (*model.ContainerActionResult, error) {
	id := strings.TrimSpace(containerID)
	if id == "" {
		return nil, fmt.Errorf("container id 不能为空")
	}

	path := "/containers/" + url.PathEscape(id) + "/" + action
	if timeoutSeconds != nil {
		query := url.Values{}
		query.Set("t", fmt.Sprintf("%d", *timeoutSeconds))
		path += "?" + query.Encode()
	}

	if _, err := s.fetchDockerAction(http.MethodPost, path, nil); err != nil {
		return nil, fmt.Errorf("docker api %s container failed: %w", action, err)
	}
	return &model.ContainerActionResult{
		ContainerID: id,
		Action:      action,
		Status:      "success",
		Message:     "ok",
		CheckedAt:   time.Now().Unix(),
	}, nil
}

// normalizeContainerTimeout 统一收口 stop/restart 的超时参数。
//
// 这次先按 old 的边界保持一致：
// - 不传时使用保守默认值；
// - 禁止负数；
// - 上限卡在 3600 秒，避免误传极大值把生命周期接口挂住。
func normalizeContainerTimeout(param *model.ContainerActionRequest, defaultSeconds int) (int, error) {
	if param == nil || param.TimeoutSeconds == nil {
		return defaultSeconds, nil
	}
	timeoutSeconds := *param.TimeoutSeconds
	if timeoutSeconds < 0 {
		return 0, fmt.Errorf("timeoutSeconds 不能小于 0")
	}
	if timeoutSeconds > 3600 {
		return 0, fmt.Errorf("timeoutSeconds 不能大于 3600")
	}
	return timeoutSeconds, nil
}

func (s *Service) fetchRaw(baseURL string, method string, path string, body io.Reader) ([]byte, error) {
	endpoint := strings.TrimRight(baseURL, "/") + path
	request, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	payload, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("status=%d body=%s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

func (s *Service) fetchDockerStream(method string, path string, body io.Reader, onLine func([]byte) error) error {
	endpoint := strings.TrimRight(currentDockerAPIURL(), "/") + path
	request, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := s.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		payload, _ := io.ReadAll(response.Body)
		return fmt.Errorf("status=%d body=%s", response.StatusCode, strings.TrimSpace(string(payload)))
	}

	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if onLine != nil {
			if err := onLine(line); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func currentDockerAPIURL() string {
	if Settings.Conf == nil || Settings.Conf.DockerConfig == nil {
		return ""
	}
	return normalizeDockerAPIURL(Settings.Conf.DockerConfig.APIURL)
}

func normalizeDockerAPIURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	return strings.TrimRight(value, "/")
}
