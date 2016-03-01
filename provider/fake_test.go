package provider

import "testing"

func TestFakeProviderCreate(t *testing.T) {
	provider := &FakeProvider{}

	_, err := provider.Create("")
	if err == nil {
		t.Errorf("expected error, got nil")
	}

	_, err = provider.Create("nonexistant-image")
	if err == nil {
		t.Errorf("expected error, got nil")
	}

	instance, err := provider.Create("standard-image")
	if err != nil {
		t.Errorf("provider.Create returned error: %v", err)
	}

	if instance.State != InstanceStateStarting {
		t.Errorf("expected state to be %v, was %v", InstanceStateStarting, instance.State)
	}
}
