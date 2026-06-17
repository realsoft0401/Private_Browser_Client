package BrowserEnv

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	packageModel "private_browser_client/Models/Package"
	slotModel "private_browser_client/Models/Slot"
	taskModel "private_browser_client/Models/Task"
	common "private_browser_client/Repository/Common"
	packageService "private_browser_client/Service/Package"
	slotService "private_browser_client/Service/Slot"
	slotRuntimeService "private_browser_client/Service/SlotRuntime"
	taskService "private_browser_client/Service/Task"
	"private_browser_client/Settings"
)

const testProxyConfigBase64 = "cG9ydDogNzg5MAptb2RlOiBydWxlCg=="

func TestCreateBuildsBrowserEnvAssetsAndIndex(t *testing.T) {
	result, err := NewService().Create(&model.CreateBrowserEnvRequest{
		UserID:  "906090001",
		RPAType: "tk",
		Name:    "create-test-env",
		Runtime: model.CreateBrowserEnvRuntime{
			Image:      "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
			StartupURL: "https://www.tiktok.com",
			ShmSize:    "1g",
		},
		Environment: model.CreateBrowserEnvEnvironment{
			Timezone: "Asia/Shanghai",
			Screen: model.CreateBrowserEnvScreen{
				Width:  1440,
				Height: 900,
				Depth:  24,
			},
		},
		Proxy: model.CreateBrowserEnvProxy{
			Enabled:      optionalBool(true),
			Type:         "clash",
			ConfigBase64: testProxyConfigBase64,
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.EnvID == "" {
		t.Fatalf("Create() envId is empty")
	}
	if result.EnvSequence <= 0 {
		t.Fatalf("Create() envSequence = %d, want > 0", result.EnvSequence)
	}

	index, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(result.EnvID)
	if err != nil {
		t.Fatalf("GetBrowserEnvIndexByID() error = %v", err)
	}
	if index.Status != model.BrowserEnvStatusCreated {
		t.Fatalf("index status = %s, want %s", index.Status, model.BrowserEnvStatusCreated)
	}
	if index.ContainerStatus != model.ContainerStatusMissing {
		t.Fatalf("index container status = %s, want %s", index.ContainerStatus, model.ContainerStatusMissing)
	}

	view, err := packageService.NewService().GetByPackageID(result.EnvID)
	if err != nil {
		t.Fatalf("GetByPackageID() error = %v", err)
	}
	if view.RuntimeStatus != packageModel.StatusCreated {
		t.Fatalf("runtime view status = %s, want %s", view.RuntimeStatus, packageModel.StatusCreated)
	}

	envPath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(result.EnvPath))
	requiredFiles := []string{
		filepath.Join(envPath, "profile.json"),
		filepath.Join(envPath, "binding.json"),
		filepath.Join(envPath, "container.json"),
		filepath.Join(envPath, "proxy", "clash.yaml"),
		filepath.Join(envPath, "proxy", "proxy-runtime.json"),
		filepath.Join(envPath, "fingerprint", "snapshot.json"),
		filepath.Join(envPath, "fingerprint", "backup.json"),
	}
	for _, file := range requiredFiles {
		if _, err = os.Stat(file); err != nil {
			t.Fatalf("expected file %s to exist: %v", file, err)
		}
	}
}

func TestUpdateProxyResetsRuntimeProtectionToPending(t *testing.T) {
	result := mustCreateBrowserEnvForTest(t, "906090002", "tk", "proxy-update-test")

	enabled := false
	response, err := NewService().UpdateProxy(result.EnvID, &model.UpdateBrowserEnvProxyRequest{
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("UpdateProxy() error = %v", err)
	}
	if response.RuntimeProtectionStatus != "pending" {
		t.Fatalf("RuntimeProtectionStatus = %s, want pending", response.RuntimeProtectionStatus)
	}
	if response.ProxyRuntimeStatus != "pending" {
		t.Fatalf("ProxyRuntimeStatus = %s, want pending", response.ProxyRuntimeStatus)
	}

	index, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(result.EnvID)
	if err != nil {
		t.Fatalf("GetBrowserEnvIndexByID() error = %v", err)
	}
	pkg, err := loadPackage(index)
	if err != nil {
		t.Fatalf("loadPackage() error = %v", err)
	}
	if pkg.Profile.Proxy.Enabled {
		t.Fatalf("profile proxy enabled = true, want false")
	}
	if pkg.Binding.RuntimeProtection.TimezoneStatus != "pending" {
		t.Fatalf("timezone status = %s, want pending", pkg.Binding.RuntimeProtection.TimezoneStatus)
	}
}

func TestRunCreatesTaskAndPublishesCompletion(t *testing.T) {
	slotRuntimeService.SetInitializer(fakeBrowserEnvSlotInitializer{})
	defer slotRuntimeService.SetInitializer(nil)
	slotRuntimeRebuilder = fakeSlotRuntimeRebuilder
	defer func() {
		slotRuntimeRebuilder = rebuildSlotRuntimeForPackage
	}()

	slotID := "slot101"
	env := mustCreateBrowserEnvForTest(t, "906090005", "tk", "run-task-test")
	envID := env.EnvID

	if _, err := slotService.NewService().CreateSlot(slotID); err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}

	result, err := NewService().Run(envID, model.RunRequest{SlotID: slotID})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.TaskID == "" {
		t.Fatalf("Run() taskId is empty")
	}

	event := waitTaskDoneEvent(t, result.TaskID)
	if event.Event != taskModel.EventCompleted {
		t.Fatalf("Run() final event = %s, want %s", event.Event, taskModel.EventCompleted)
	}
	if event.ResourceID != envID {
		t.Fatalf("Run() resource id = %s, want %s", event.ResourceID, envID)
	}
}

func TestBackupAndRestoreRoundTrip(t *testing.T) {
	result := mustCreateBrowserEnvForTest(t, "906090003", "tk", "backup-restore-test")

	accepted, err := NewService().Backup(result.EnvID)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	event := waitTaskDoneEvent(t, accepted.TaskID)
	if event.Event != taskModel.EventCompleted {
		t.Fatalf("Backup() final event = %s, want %s", event.Event, taskModel.EventCompleted)
	}

	index, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(result.EnvID)
	if err != nil {
		t.Fatalf("GetBrowserEnvIndexByID() after backup error = %v", err)
	}
	if index.Status != model.BrowserEnvStatusBackedUp {
		t.Fatalf("index status after backup = %s, want %s", index.Status, model.BrowserEnvStatusBackedUp)
	}
	if index.BackupPath == nil || *index.BackupPath == "" {
		t.Fatalf("backup path is empty after backup")
	}
	envPath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(result.EnvPath))
	if _, err = os.Stat(envPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("env path should be removed after backup, stat err = %v", err)
	}

	restoreAccepted, err := NewService().Restore(result.EnvID)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	restoreEvent := waitTaskDoneEvent(t, restoreAccepted.TaskID)
	if restoreEvent.Event != taskModel.EventCompleted {
		t.Fatalf("Restore() final event = %s, want %s", restoreEvent.Event, taskModel.EventCompleted)
	}

	index, err = browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(result.EnvID)
	if err != nil {
		t.Fatalf("GetBrowserEnvIndexByID() after restore error = %v", err)
	}
	if index.Status != model.BrowserEnvStatusCreated {
		t.Fatalf("index status after restore = %s, want %s", index.Status, model.BrowserEnvStatusCreated)
	}
	if index.BackupPath != nil {
		t.Fatalf("backup path should be cleared after restore")
	}
	if _, err = os.Stat(envPath); err != nil {
		t.Fatalf("env path should exist after restore: %v", err)
	}
}

func TestStopWithoutActiveRelationReturnsStopped(t *testing.T) {
	envID := "906090001_tk_stop_idle"

	result, err := NewService().Stop(envID, model.StopRequest{})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if result.Status != packageModel.StatusStopped {
		t.Fatalf("Stop() status = %s, want %s", result.Status, packageModel.StatusStopped)
	}
	if result.ContainerStatus != "missing" {
		t.Fatalf("Stop() container status = %s, want missing", result.ContainerStatus)
	}
}

func TestRevalidateClearsErrorStateWhenAssetsAreValid(t *testing.T) {
	result := mustCreateBrowserEnvForTest(t, "906090004", "tk", "revalidate-test")
	now := time.Now().Unix()
	if err := browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(&model.BrowserEnvRuntimeUpdate{
		EnvID:           result.EnvID,
		Status:          model.BrowserEnvStatusError,
		ContainerStatus: model.ContainerStatusError,
		MonitorStatus:   model.MonitorStatusUnknown,
		LastError:       optionalTestString("synthetic test error"),
		UpdatedAt:       now,
		LastCheckedAt:   &now,
	}); err != nil {
		t.Fatalf("UpdateBrowserEnvRuntime() error = %v", err)
	}

	accepted, err := NewService().Revalidate(result.EnvID)
	if err != nil {
		t.Fatalf("Revalidate() error = %v", err)
	}
	event := waitTaskDoneEvent(t, accepted.TaskID)
	if event.Event != taskModel.EventCompleted {
		t.Fatalf("Revalidate() final event = %s, want %s", event.Event, taskModel.EventCompleted)
	}

	index, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(result.EnvID)
	if err != nil {
		t.Fatalf("GetBrowserEnvIndexByID() error = %v", err)
	}
	if index.Status != model.BrowserEnvStatusCreated {
		t.Fatalf("index status after revalidate = %s, want %s", index.Status, model.BrowserEnvStatusCreated)
	}
	if index.LastError != nil {
		t.Fatalf("last error should be cleared after revalidate")
	}
}

func TestDeletePackageRejectsRunningRelation(t *testing.T) {
	slotRuntimeService.SetInitializer(fakeBrowserEnvSlotInitializer{})
	defer slotRuntimeService.SetInitializer(nil)
	slotRuntimeRebuilder = fakeSlotRuntimeRebuilder
	defer func() {
		slotRuntimeRebuilder = rebuildSlotRuntimeForPackage
	}()

	slotID := "slot102"
	env := mustCreateBrowserEnvForTest(t, "906090006", "tk", "delete-conflict-test")
	envID := env.EnvID

	if _, err := slotService.NewService().CreateSlot(slotID); err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}
	if _, err := NewService().Run(envID, model.RunRequest{SlotID: slotID}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if _, err := NewService().DeletePackage(envID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("DeletePackage() error = %v, want conflict", err)
	}
}

func mustCreateBrowserEnvForTest(t *testing.T, userID string, rpaType string, name string) *model.CreateBrowserEnvResponse {
	t.Helper()

	result, err := NewService().Create(&model.CreateBrowserEnvRequest{
		UserID:  userID,
		RPAType: rpaType,
		Name:    name,
		Runtime: model.CreateBrowserEnvRuntime{
			Image:      "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
			StartupURL: "https://www.tiktok.com",
			ShmSize:    "1g",
		},
		Environment: model.CreateBrowserEnvEnvironment{
			Timezone: "Asia/Shanghai",
			Screen: model.CreateBrowserEnvScreen{
				Width:  1440,
				Height: 900,
				Depth:  24,
			},
		},
		Proxy: model.CreateBrowserEnvProxy{
			Enabled:      optionalBool(true),
			Type:         "clash",
			ConfigBase64: testProxyConfigBase64,
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return result
}

func waitTaskDoneEvent(t *testing.T, taskID string) taskModel.Event {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, _, _, err := taskService.GetService().Subscribe(taskID)
		if err == nil && len(snapshot.Events) > 0 {
			last := snapshot.Events[len(snapshot.Events)-1]
			if last.Event == taskModel.EventCompleted || last.Event == taskModel.EventFailed {
				return last
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s did not finish in time", taskID)
	return taskModel.Event{}
}

type fakeBrowserEnvSlotInitializer struct{}

func (f fakeBrowserEnvSlotInitializer) Initialize(slot *slotModel.Slot) error {
	if slot == nil {
		return nil
	}
	slot.ContainerID = optionalTestString("fake-container-id")
	slot.ContainerName = optionalTestString("fake-container-name")
	slot.RuntimeImage = optionalTestString("fake-image")
	slot.ContainerStatus = optionalTestString("running")
	return nil
}

func (f fakeBrowserEnvSlotInitializer) Destroy(slot *slotModel.Slot) error {
	_ = slot
	return nil
}

func (f fakeBrowserEnvSlotInitializer) Reinitialize(slot *slotModel.Slot) error {
	return f.Initialize(slot)
}

func fakeSlotRuntimeRebuilder(slot *slotModel.Slot, _ *loadedPackage) (*edgeModel.DockerContainerCreateResult, error) {
	if slot == nil {
		return &edgeModel.DockerContainerCreateResult{ID: "fake-run-container-id"}, nil
	}
	slot.ContainerID = optionalTestString("fake-run-container-id")
	slot.ContainerName = optionalTestString("fake-run-container-name")
	slot.RuntimeImage = optionalTestString("fake-run-image")
	slot.ContainerStatus = optionalTestString(model.ContainerStatusRunning)
	slot.LastError = nil
	return &edgeModel.DockerContainerCreateResult{ID: "fake-run-container-id"}, nil
}

func optionalTestString(value string) *string {
	return &value
}

func optionalBool(value bool) *bool {
	return &value
}
