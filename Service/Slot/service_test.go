package Slot

import (
	"testing"

	model "private_browser_client/Models/Slot"
	slotRuntimeService "private_browser_client/Service/SlotRuntime"
)

func TestCreateSlot(t *testing.T) {
	slotRuntimeService.SetInitializer(fakeSlotInitializer{})
	defer slotRuntimeService.SetInitializer(nil)

	service := NewService()
	slotID := "slot301"

	slot, err := service.CreateSlot(slotID)
	if err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}
	if slot.SlotID != slotID {
		t.Fatalf("CreateSlot() slot id = %s, want %s", slot.SlotID, slotID)
	}
	if slot.Status != model.StatusWaiting {
		t.Fatalf("CreateSlot() status = %s, want %s", slot.Status, model.StatusWaiting)
	}
}

func TestCreateSlotRejectsDuplicate(t *testing.T) {
	slotRuntimeService.SetInitializer(fakeSlotInitializer{})
	defer slotRuntimeService.SetInitializer(nil)

	service := NewService()
	slotID := "slot302"

	if _, err := service.CreateSlot(slotID); err != nil {
		t.Fatalf("CreateSlot(first) error = %v", err)
	}
	if _, err := service.CreateSlot(slotID); err == nil {
		t.Fatalf("CreateSlot(second) error = nil, want duplicate conflict")
	}
}

func TestGetSlotVNCInfo(t *testing.T) {
	slotRuntimeService.SetInitializer(fakeSlotInitializer{})
	defer slotRuntimeService.SetInitializer(nil)

	service := NewService()
	slotID := "slot303"

	slot, err := service.CreateSlot(slotID)
	if err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}

	slot.ContainerStatus = optionalModelString("running")
	slot.VNCPort = optionalModelInt(9101)
	slot.CDPPort = optionalModelInt(9201)
	if err = service.UpdateSlot(slot); err != nil {
		t.Fatalf("UpdateSlot() error = %v", err)
	}

	result, err := service.GetSlotVNCInfo(slotID, "http://127.0.0.1:3300", "ws://127.0.0.1:3300")
	if err != nil {
		t.Fatalf("GetSlotVNCInfo() error = %v", err)
	}
	if result.SlotID != slotID {
		t.Fatalf("GetSlotVNCInfo() slot id = %s, want %s", result.SlotID, slotID)
	}
	if result.VNCPort != 9101 {
		t.Fatalf("GetSlotVNCInfo() vnc port = %d, want 9101", result.VNCPort)
	}
	if result.WSURL == "" {
		t.Fatalf("GetSlotVNCInfo() wsUrl is empty")
	}
}

func TestGetSlotCDPInfo(t *testing.T) {
	slotRuntimeService.SetInitializer(fakeSlotInitializer{})
	defer slotRuntimeService.SetInitializer(nil)

	service := NewService()
	slotID := "slot304"

	slot, err := service.CreateSlot(slotID)
	if err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}

	slot.ContainerStatus = optionalModelString("running")
	slot.CDPPort = optionalModelInt(9201)
	if err = service.UpdateSlot(slot); err != nil {
		t.Fatalf("UpdateSlot() error = %v", err)
	}

	result, err := service.GetSlotCDPInfo(slotID, "http://127.0.0.1:3300")
	if err != nil {
		t.Fatalf("GetSlotCDPInfo() error = %v", err)
	}
	if result.SlotID != slotID {
		t.Fatalf("GetSlotCDPInfo() slot id = %s, want %s", result.SlotID, slotID)
	}
	if result.CDPPort != 9201 {
		t.Fatalf("GetSlotCDPInfo() cdp port = %d, want 9201", result.CDPPort)
	}
	if result.VersionURL == "" {
		t.Fatalf("GetSlotCDPInfo() versionUrl is empty")
	}
}

type fakeSlotInitializer struct{}

func (f fakeSlotInitializer) Initialize(slot *model.Slot) error {
	if slot == nil {
		return nil
	}
	slot.ContainerID = optionalModelString("fake-container-id")
	slot.ContainerName = optionalModelString("fake-container-name")
	slot.RuntimeImage = optionalModelString("fake-image")
	slot.ContainerStatus = optionalModelString("running")
	return nil
}

func (f fakeSlotInitializer) Destroy(slot *model.Slot) error {
	_ = slot
	return nil
}

func (f fakeSlotInitializer) Reinitialize(slot *model.Slot) error {
	return f.Initialize(slot)
}

func optionalModelString(value string) *string {
	return &value
}

func optionalModelInt(value int) *int {
	return &value
}
