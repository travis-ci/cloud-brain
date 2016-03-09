package cloud

import "testing"

// Ensure that FakeProvider implements the Provider interface
var _ Provider = &FakeProvider{}

func TestFakeProviderCreate(t *testing.T) {
	provider := &FakeProvider{}

	_, err := provider.Create(CreateAttributes{ImageName: ""})
	if err == nil {
		t.Errorf("expected error, got nil")
	}

	_, err = provider.Create(CreateAttributes{ImageName: "nonexistant-image"})
	if err == nil {
		t.Errorf("expected error, got nil")
	}

	instance, err := provider.Create(CreateAttributes{ImageName: "standard-image"})
	if err != nil {
		t.Errorf("provider.Create returned error: %v", err)
	}

	if instance.State != InstanceStateStarting {
		t.Errorf("expected state to be %v, was %v", InstanceStateStarting, instance.State)
	}
}
