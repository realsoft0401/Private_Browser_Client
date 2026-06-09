package BrowserEnv

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

var (
	statusSyncMu         sync.Mutex
	statusSyncManager    *StatusSyncManager
	errStatusSyncSkipped = errors.New("status sync skipped")
)

type statusSyncRuntimeConfig struct {
	enabled          bool
	interval         time.Duration
	watchdogInterval time.Duration
	staleAfter       time.Duration
}

// StatusSyncManager 管理浏览器环境运行态同步 Worker 和哨兵。
//
// 设计来源：
// - 用户要求边缘服务每隔几秒主动获取 Docker 容器状态并刷新 SQLite；
// - 用户特别提醒“定时任务挂了要自动拉起来”，所以这里不能只启动一个裸 goroutine；
// - Manager 负责生命周期、心跳、重启次数和健康快照，真正业务同步放在 Worker 的单轮执行里。
//
// 职责边界：
// - 只守护“状态同步任务”本身，不守护浏览器容器；
// - 不自动启动、停止、删除浏览器容器；
// - 不修改代理、指纹、profile 或 browser-data 登录态目录。
type StatusSyncManager struct {
	config statusSyncRuntimeConfig

	mu           sync.Mutex
	stopCh       chan struct{}
	doneCh       chan struct{}
	workerCancel context.CancelFunc
	workerDone   chan struct{}
	generation   int64

	workerRunning     bool
	restarts          int64
	lastStartedAt     int64
	lastHeartbeatAt   int64
	lastFinishedAt    int64
	lastSuccessAt     int64
	lastError         string
	lastPanic         string
	lastSkippedReason string
	lastScannedCount  int
	lastUpdatedCount  int
}

// StartStatusSyncManager 启动全局后台状态同步管理器。
//
// 它由基础设施层在 SQLite 初始化完成后调用；如果配置关闭，则只保留一个 disabled 快照。
// 这里使用全局单例，是因为当前边缘服务进程只有一个本机 SQLite 和一个本机 Docker API。
func StartStatusSyncManager() *StatusSyncManager {
	statusSyncMu.Lock()
	defer statusSyncMu.Unlock()
	if statusSyncManager != nil {
		return statusSyncManager
	}

	manager := NewStatusSyncManager(Settings.Conf.StatusSyncConfig)
	statusSyncManager = manager
	manager.Start()
	return manager
}

// StopStatusSyncManager 停止全局后台状态同步管理器。
//
// 基础设施层应先停任务再关闭 SQLite，避免 Worker 退出时继续写已关闭的数据库连接。
func StopStatusSyncManager() {
	statusSyncMu.Lock()
	manager := statusSyncManager
	statusSyncManager = nil
	statusSyncMu.Unlock()
	if manager != nil {
		manager.Stop()
	}
}

// StatusSyncSnapshot 返回当前后台同步任务健康快照。
//
// /health 可以通过它暴露任务状态；如果 Manager 尚未启动，返回 disabled 快照，避免健康接口 panic。
func StatusSyncSnapshot() model.StatusSyncSnapshot {
	statusSyncMu.Lock()
	manager := statusSyncManager
	statusSyncMu.Unlock()
	if manager == nil {
		return model.StatusSyncSnapshot{Enabled: false}
	}
	return manager.Snapshot()
}

// NewStatusSyncManager 根据配置创建状态同步 Manager。
//
// 配置已经在 Settings 层做过下限归一化，这里再次兜底，是为了让测试或未来手工构造 Manager 时也安全。
func NewStatusSyncManager(config *Settings.StatusSyncConfig) *StatusSyncManager {
	runtimeConfig := buildStatusSyncRuntimeConfig(config)
	return &StatusSyncManager{
		config: runtimeConfig,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start 启动 Worker 和 Watchdog。
//
// Start 只允许调用一次；如果 status_sync.enabled=false，则不启动 goroutine。
func (m *StatusSyncManager) Start() {
	if m == nil {
		return
	}
	if !m.config.enabled {
		close(m.doneCh)
		return
	}
	m.mu.Lock()
	m.startWorkerLocked(false)
	m.mu.Unlock()

	go m.watchdogLoop()
}

// Stop 停止 Worker 和 Watchdog。
//
// Stop 不负责停止任何浏览器容器；它只收束后台状态同步任务自身。
func (m *StatusSyncManager) Stop() {
	if m == nil {
		return
	}
	select {
	case <-m.doneCh:
		return
	default:
	}
	close(m.stopCh)
	m.mu.Lock()
	if m.workerCancel != nil {
		m.workerCancel()
	}
	workerDone := m.workerDone
	m.mu.Unlock()
	if workerDone != nil {
		select {
		case <-workerDone:
		case <-time.After(5 * time.Second):
			log.Printf("browser env status sync worker stop timeout\n")
		}
	}
	<-m.doneCh
}

// Snapshot 返回 Manager 当前可观测状态。
//
// 这个快照不读数据库、不访问 Docker，只展示任务自身健康度和最近一轮统计。
func (m *StatusSyncManager) Snapshot() model.StatusSyncSnapshot {
	if m == nil {
		return model.StatusSyncSnapshot{Enabled: false}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return model.StatusSyncSnapshot{
		Enabled:           m.config.enabled,
		WorkerRunning:     m.workerRunning,
		Restarts:          m.restarts,
		IntervalSeconds:   int(m.config.interval.Seconds()),
		WatchdogSeconds:   int(m.config.watchdogInterval.Seconds()),
		StaleSeconds:      int(m.config.staleAfter.Seconds()),
		LastStartedAt:     int64Ptr(m.lastStartedAt),
		LastHeartbeatAt:   int64Ptr(m.lastHeartbeatAt),
		LastFinishedAt:    int64Ptr(m.lastFinishedAt),
		LastSuccessAt:     int64Ptr(m.lastSuccessAt),
		LastError:         stringPtr(m.lastError),
		LastPanic:         stringPtr(m.lastPanic),
		LastSkippedReason: stringPtr(m.lastSkippedReason),
		LastScannedCount:  m.lastScannedCount,
		LastUpdatedCount:  m.lastUpdatedCount,
	}
}

func buildStatusSyncRuntimeConfig(config *Settings.StatusSyncConfig) statusSyncRuntimeConfig {
	if config == nil {
		config = &Settings.StatusSyncConfig{
			Enabled:         true,
			IntervalSeconds: 5,
			WatchdogSeconds: 15,
			StaleSeconds:    30,
		}
	}
	interval := time.Duration(config.IntervalSeconds) * time.Second
	if interval < 3*time.Second {
		interval = 5 * time.Second
	}
	watchdogInterval := time.Duration(config.WatchdogSeconds) * time.Second
	if watchdogInterval < 5*time.Second {
		watchdogInterval = 15 * time.Second
	}
	staleAfter := time.Duration(config.StaleSeconds) * time.Second
	if staleAfter < watchdogInterval*2 {
		staleAfter = watchdogInterval * 2
	}
	return statusSyncRuntimeConfig{
		enabled:          config.Enabled,
		interval:         interval,
		watchdogInterval: watchdogInterval,
		staleAfter:       staleAfter,
	}
}

func (m *StatusSyncManager) startWorkerLocked(isRestart bool) {
	now := time.Now().Unix()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.generation++
	generation := m.generation
	m.workerCancel = cancel
	m.workerDone = done
	m.workerRunning = true
	m.lastStartedAt = now
	m.lastHeartbeatAt = now
	m.lastPanic = ""
	m.lastSkippedReason = ""
	if isRestart {
		m.restarts++
	}
	go m.workerLoop(ctx, generation, done)
	log.Printf("browser env status sync worker started, generation=%d\n", generation)
}

func (m *StatusSyncManager) watchdogLoop() {
	defer close(m.doneCh)
	ticker := time.NewTicker(m.config.watchdogInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkWorker()
		}
	}
}

func (m *StatusSyncManager) checkWorker() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.config.enabled {
		return
	}
	if !m.workerRunning {
		m.startWorkerLocked(true)
		return
	}
	if time.Since(time.Unix(m.lastHeartbeatAt, 0)) <= m.config.staleAfter {
		return
	}
	m.lastError = fmt.Sprintf("status sync worker heartbeat stale over %s", m.config.staleAfter)
	if m.workerCancel != nil {
		m.workerCancel()
	}
	m.workerRunning = false
	m.startWorkerLocked(true)
}

func (m *StatusSyncManager) workerLoop(ctx context.Context, generation int64, done chan struct{}) {
	defer close(done)
	defer func() {
		if recovered := recover(); recovered != nil {
			m.recordPanic(generation, fmt.Sprintf("%v\n%s", recovered, string(debug.Stack())))
		}
		m.markWorkerStopped(generation)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		scanned, updated, err := m.runOnce(ctx, generation)
		m.recordRunResult(generation, scanned, updated, err)

		timer := time.NewTimer(m.config.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *StatusSyncManager) runOnce(ctx context.Context, generation int64) (int, int, error) {
	m.recordHeartbeat(generation)
	if !runEnvMu.TryLock() {
		m.recordSkipped(generation, "lifecycle lock busy")
		return 0, 0, errStatusSyncSkipped
	}
	defer runEnvMu.Unlock()

	if !m.isCurrentGeneration(generation) {
		return 0, 0, nil
	}
	handler := browserEnvDao.NewStatusSyncModelHandler()
	targets, err := handler.ListStatusSyncTargets(ctx)
	if err != nil {
		return 0, 0, err
	}
	containers, err := edgeService.NewEdgeService().GetDockerContainers()
	if err != nil {
		return len(targets), 0, err
	}
	lookup := buildStatusSyncContainerLookup(containers)

	updated := 0
	var lastErr error
	for _, target := range targets {
		if target == nil || !m.isCurrentGeneration(generation) {
			continue
		}
		changed, syncErr := syncOneBrowserEnvStatus(ctx, handler, target, lookup)
		if syncErr != nil {
			lastErr = syncErr
			log.Printf("browser env status sync failed, envId=%s, err=%v\n", target.EnvID, syncErr)
			continue
		}
		if changed {
			updated++
		}
	}
	return len(targets), updated, lastErr
}

func (m *StatusSyncManager) isCurrentGeneration(generation int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.generation == generation
}

func (m *StatusSyncManager) recordHeartbeat(generation int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != generation {
		return
	}
	m.lastHeartbeatAt = time.Now().Unix()
}

func (m *StatusSyncManager) recordSkipped(generation int64, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != generation {
		return
	}
	m.lastSkippedReason = reason
	m.lastHeartbeatAt = time.Now().Unix()
}

func (m *StatusSyncManager) recordRunResult(generation int64, scanned int, updated int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != generation {
		return
	}
	now := time.Now().Unix()
	m.lastHeartbeatAt = now
	m.lastFinishedAt = now
	m.lastScannedCount = scanned
	m.lastUpdatedCount = updated
	if errors.Is(err, errStatusSyncSkipped) {
		return
	}
	if err != nil {
		m.lastError = err.Error()
		return
	}
	m.lastError = ""
	m.lastSkippedReason = ""
	m.lastSuccessAt = now
}

func (m *StatusSyncManager) recordPanic(generation int64, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != generation {
		return
	}
	m.lastPanic = truncateStatusSyncMessage(message)
	m.lastError = "status sync worker panic"
}

func (m *StatusSyncManager) markWorkerStopped(generation int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != generation {
		return
	}
	m.workerRunning = false
}

type statusSyncContainerLookup struct {
	byID   map[string]edgeModel.DockerContainer
	byName map[string]edgeModel.DockerContainer
}

func buildStatusSyncContainerLookup(containers []edgeModel.DockerContainer) statusSyncContainerLookup {
	lookup := statusSyncContainerLookup{
		byID:   map[string]edgeModel.DockerContainer{},
		byName: map[string]edgeModel.DockerContainer{},
	}
	for _, container := range containers {
		if strings.TrimSpace(container.ID) != "" {
			lookup.byID[container.ID] = container
		}
		for _, name := range container.Names {
			name = strings.TrimPrefix(strings.TrimSpace(name), "/")
			if name != "" {
				lookup.byName[name] = container
			}
		}
	}
	return lookup
}

func syncOneBrowserEnvStatus(ctx context.Context, handler *browserEnvDao.StatusSyncModelHandler, index *model.BrowserEnvIndex, lookup statusSyncContainerLookup) (bool, error) {
	pkg, err := loadStatusSyncPackage(index)
	if err != nil {
		return syncStatusPackageLoadError(ctx, handler, index, err)
	}
	containerID, containerName := resolveContainerIdentity(index, pkg.Container)
	found, exists := findStatusSyncContainer(containerID, containerName, lookup)
	next := buildStatusSyncUpdate(index, pkg, found, exists)
	if !next.dbChanged && !next.filesChanged {
		return false, nil
	}
	if next.filesChanged {
		if err = writePackageJSON(pkg.AbsoluteEnvPath, pkg.Profile.Paths.Container, pkg.Container); err != nil {
			return false, err
		}
		if err = writePackageJSON(pkg.AbsoluteEnvPath, pkg.Profile.Paths.Profile, pkg.Profile); err != nil {
			return false, err
		}
	}
	if next.dbChanged {
		return true, handler.UpdateBrowserEnvRuntime(ctx, next.runtime)
	}
	return next.filesChanged, nil
}

type statusSyncPackage struct {
	Profile         model.ProfileFile
	Container       model.ContainerFile
	AbsoluteEnvPath string
}

func loadStatusSyncPackage(index *model.BrowserEnvIndex) (*statusSyncPackage, error) {
	absoluteEnvPath, profile, err := loadPackageProfileFromIndex(index)
	if err != nil {
		return nil, err
	}
	var container model.ContainerFile
	if err := readPackageJSON(absoluteEnvPath, profile.Paths.Container, &container); err != nil {
		return nil, err
	}
	return &statusSyncPackage{
		Profile:         profile,
		Container:       container,
		AbsoluteEnvPath: absoluteEnvPath,
	}, nil
}

func findStatusSyncContainer(containerID string, containerName string, lookup statusSyncContainerLookup) (edgeModel.DockerContainer, bool) {
	if strings.TrimSpace(containerID) != "" {
		if container, ok := lookup.byID[strings.TrimSpace(containerID)]; ok {
			return container, true
		}
		for id, container := range lookup.byID {
			if strings.HasPrefix(id, strings.TrimSpace(containerID)) {
				return container, true
			}
		}
	}
	if strings.TrimSpace(containerName) != "" {
		if container, ok := lookup.byName[strings.TrimSpace(containerName)]; ok {
			return container, true
		}
	}
	return edgeModel.DockerContainer{}, false
}

type statusSyncUpdate struct {
	runtime      *model.BrowserEnvRuntimeUpdate
	dbChanged    bool
	filesChanged bool
}

func buildStatusSyncUpdate(index *model.BrowserEnvIndex, pkg *statusSyncPackage, found edgeModel.DockerContainer, exists bool) statusSyncUpdate {
	now := time.Now().Unix()
	containerID, containerName := resolveContainerIdentity(index, pkg.Container)
	nextStatus := index.Status
	nextContainerStatus := index.ContainerStatus
	var lastError *string
	lastStartedAt := index.LastStartedAt
	lastStoppedAt := index.LastStoppedAt

	if exists {
		containerID = found.ID
		if name := firstContainerName(found, containerName); name != "" {
			containerName = name
		}
		nextStatus, nextContainerStatus, lastError = classifyDockerContainerState(found.State)
		observedStatus := nextStatus
		if index.Status == model.BrowserEnvStatusError {
			nextStatus = model.BrowserEnvStatusError
			lastError = preserveStatusSyncError(index.LastError)
		}
		if observedStatus == model.BrowserEnvStatusRunning && lastStartedAt == nil {
			lastStartedAt = &now
		}
		if observedStatus != model.BrowserEnvStatusRunning && lastStoppedAt == nil {
			lastStoppedAt = &now
		}
		pkg.Container.Image = found.Image
	} else if index.Status == model.BrowserEnvStatusCreated && strings.TrimSpace(containerID) == "" {
		nextStatus = model.BrowserEnvStatusCreated
		nextContainerStatus = model.BrowserEnvContainerStatusUnknown
	} else if index.Status == model.BrowserEnvStatusError {
		nextStatus = model.BrowserEnvStatusError
		nextContainerStatus = "missing"
		lastError = preserveStatusSyncError(index.LastError)
		if lastStoppedAt == nil {
			lastStoppedAt = &now
		}
	} else {
		nextStatus = model.BrowserEnvStatusStopped
		nextContainerStatus = "missing"
		message := "container not found by status sync"
		lastError = &message
		if lastStoppedAt == nil {
			lastStoppedAt = &now
		}
	}

	filesChanged := pkg.Container.Status != nextStatus ||
		stringValue(pkg.Container.ContainerID) != strings.TrimSpace(containerID) ||
		strings.TrimSpace(pkg.Container.ContainerName) != strings.TrimSpace(containerName) ||
		int64Value(pkg.Container.StartedAt) != int64Value(lastStartedAt) ||
		int64Value(pkg.Container.StoppedAt) != int64Value(lastStoppedAt)

	pkg.Container.ContainerID = optionalString(containerID)
	if strings.TrimSpace(containerName) != "" {
		pkg.Container.ContainerName = containerName
	}
	pkg.Container.Status = nextStatus
	pkg.Container.StartedAt = lastStartedAt
	pkg.Container.StoppedAt = lastStoppedAt
	if filesChanged {
		pkg.Container.UpdatedAt = now
	}
	if strings.TrimSpace(pkg.Container.Docker.APIURL) == "" {
		pkg.Container.Docker.APIURL = Settings.Conf.DockerConfig.APIURL
	}

	profileRuntimeChanged := stringValue(pkg.Profile.LastRuntime.ContainerID) != strings.TrimSpace(containerID) ||
		stringValue(pkg.Profile.LastRuntime.ContainerName) != strings.TrimSpace(containerName) ||
		int64Value(pkg.Profile.LastRuntime.LastStartedAt) != int64Value(lastStartedAt) ||
		int64Value(pkg.Profile.LastRuntime.LastStoppedAt) != int64Value(lastStoppedAt)
	pkg.Profile.LastRuntime.ContainerID = optionalString(containerID)
	pkg.Profile.LastRuntime.ContainerName = optionalString(containerName)
	pkg.Profile.LastRuntime.LastStartedAt = lastStartedAt
	pkg.Profile.LastRuntime.LastStoppedAt = lastStoppedAt
	if packageLastRuntimeDockerAPIURL(pkg.Profile.LastRuntime) == "" {
		dockerAPIURL := Settings.Conf.DockerConfig.APIURL
		pkg.Profile.LastRuntime.DockerAPIURL = optionalString(dockerAPIURL)
		profileRuntimeChanged = true
	}
	filesChanged = filesChanged || profileRuntimeChanged
	if filesChanged {
		pkg.Profile.Metadata.UpdatedAt = now
	}

	lastCheckedAt := &now
	runtime := &model.BrowserEnvRuntimeUpdate{
		EnvID:           index.EnvID,
		Status:          nextStatus,
		ContainerID:     optionalString(containerID),
		ContainerName:   optionalString(containerName),
		ContainerStatus: nextContainerStatus,
		MonitorStatus:   index.MonitorStatus,
		LastError:       lastError,
		UpdatedAt:       now,
		LastStartedAt:   lastStartedAt,
		LastStoppedAt:   lastStoppedAt,
		LastCheckedAt:   lastCheckedAt,
	}
	dbChanged := index.Status != runtime.Status ||
		stringValue(index.ContainerID) != stringValue(runtime.ContainerID) ||
		stringValue(index.ContainerName) != stringValue(runtime.ContainerName) ||
		index.ContainerStatus != runtime.ContainerStatus ||
		stringValue(index.LastError) != stringValue(runtime.LastError) ||
		int64Value(index.LastCheckedAt) != now ||
		int64Value(index.LastStartedAt) != int64Value(runtime.LastStartedAt) ||
		int64Value(index.LastStoppedAt) != int64Value(runtime.LastStoppedAt)
	return statusSyncUpdate{runtime: runtime, dbChanged: dbChanged, filesChanged: filesChanged}
}

// preserveStatusSyncError 保留 error 生命周期的异常原因。
//
// 设计来源：
// - 用户确认 container_status=running 不能代表环境可用，run 探测失败后的 status=error 必须由 revalidate 解除；
// - 后台同步任务只能刷新 Docker 事实，不能因为看到容器 running/exited 就把异常环境改回正常生命周期；
// - 因此 error 状态下 last_error 优先沿用原始失败原因，缺失时写清楚需要管理员 revalidate。
func preserveStatusSyncError(current *string) *string {
	if current != nil && strings.TrimSpace(*current) != "" {
		return current
	}
	message := "环境包处于 error，后台状态同步只刷新 Docker 事实，不能解除异常；请管理员排查后调用 revalidate"
	return &message
}

func classifyDockerContainerState(state string) (string, string, *string) {
	normalized := strings.ToLower(strings.TrimSpace(state))
	switch normalized {
	case "running", "restarting":
		return model.BrowserEnvStatusRunning, normalized, nil
	case "dead":
		message := "docker container state=dead"
		return model.BrowserEnvStatusError, normalized, &message
	case "created", "exited", "paused":
		return model.BrowserEnvStatusStopped, normalized, nil
	default:
		if normalized == "" {
			normalized = model.BrowserEnvContainerStatusUnknown
		}
		return model.BrowserEnvStatusStopped, normalized, nil
	}
}

func syncStatusPackageLoadError(ctx context.Context, handler *browserEnvDao.StatusSyncModelHandler, index *model.BrowserEnvIndex, cause error) (bool, error) {
	now := time.Now().Unix()
	message := truncateStatusSyncMessage(cause.Error())
	lastCheckedAt := &now
	update := &model.BrowserEnvRuntimeUpdate{
		EnvID:           index.EnvID,
		Status:          model.BrowserEnvStatusError,
		ContainerID:     index.ContainerID,
		ContainerName:   index.ContainerName,
		ContainerStatus: index.ContainerStatus,
		MonitorStatus:   index.MonitorStatus,
		LastError:       &message,
		UpdatedAt:       now,
		LastStartedAt:   index.LastStartedAt,
		LastStoppedAt:   index.LastStoppedAt,
		LastCheckedAt:   lastCheckedAt,
	}
	if err := handler.UpdateBrowserEnvRuntime(ctx, update); err != nil {
		return false, err
	}
	return true, cause
}

func firstContainerName(container edgeModel.DockerContainer, fallback string) string {
	for _, name := range container.Names {
		name = strings.TrimPrefix(strings.TrimSpace(name), "/")
		if name != "" {
			return name
		}
	}
	return strings.TrimSpace(fallback)
}

func truncateStatusSyncMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 500 {
		return message
	}
	return message[:500]
}

func int64Ptr(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func packageLastRuntimeDockerAPIURL(runtime model.PackageLastRuntime) string {
	if runtime.DockerAPIURL == nil {
		return ""
	}
	return strings.TrimSpace(*runtime.DockerAPIURL)
}
