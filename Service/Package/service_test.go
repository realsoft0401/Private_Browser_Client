package Package

import (
	"testing"

	model "private_browser_client/Models/Package"
	slotModel "private_browser_client/Models/Slot"
	slotService "private_browser_client/Service/Slot"
	slotRuntimeService "private_browser_client/Service/SlotRuntime"
)

func TestRunAndStopPackage(t *testing.T) {
	slotRuntimeService.SetInitializer(fakePackageSlotInitializer{})
	defer slotRuntimeService.SetInitializer(nil)

	slotID := "slot201"
	packageID := "pkg-test-run-stop"

	slotSvc := slotService.NewService()
	if _, err := slotSvc.CreateSlot(slotID); err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}

	service := NewService()
	runningView, err := service.RunPackage(packageID, slotID)
	if err != nil {
		t.Fatalf("RunPackage() error = %v", err)
	}
	if runningView.RuntimeStatus != model.StatusRunning {
		t.Fatalf("RunPackage() status = %s, want %s", runningView.RuntimeStatus, model.StatusRunning)
	}
	if runningView.CurrentSlotID == nil || *runningView.CurrentSlotID != slotID {
		t.Fatalf("RunPackage() current slot = %v, want %s", runningView.CurrentSlotID, slotID)
	}

	stoppedView, err := service.StopPackage(packageID, slotID)
	if err != nil {
		t.Fatalf("StopPackage() error = %v", err)
	}
	if stoppedView.RuntimeStatus != model.StatusStopped {
		t.Fatalf("StopPackage() status = %s, want %s", stoppedView.RuntimeStatus, model.StatusStopped)
	}
	if stoppedView.CurrentSlotID != nil {
		t.Fatalf("StopPackage() current slot = %v, want nil", stoppedView.CurrentSlotID)
	}
}

func TestRunPackageRejectsOccupiedSlot(t *testing.T) {
	slotRuntimeService.SetInitializer(fakePackageSlotInitializer{})
	defer slotRuntimeService.SetInitializer(nil)

	slotID := "slot202"
	firstPackageID := "pkg-test-first"
	secondPackageID := "pkg-test-second"

	slotSvc := slotService.NewService()
	if _, err := slotSvc.CreateSlot(slotID); err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}

	service := NewService()
	if _, err := service.RunPackage(firstPackageID, slotID); err != nil {
		t.Fatalf("RunPackage(first) error = %v", err)
	}
	if _, err := service.RunPackage(secondPackageID, slotID); err == nil {
		t.Fatalf("RunPackage(second) error = nil, want conflict")
	}
}

type fakePackageSlotInitializer struct{}

func (f fakePackageSlotInitializer) Initialize(slot *slotModel.Slot) error {
	_ = slot
	return nil
}

func (f fakePackageSlotInitializer) Destroy(slot *slotModel.Slot) error {
	_ = slot
	return nil
}

func (f fakePackageSlotInitializer) Reinitialize(slot *slotModel.Slot) error {
	_ = slot
	return nil
}
