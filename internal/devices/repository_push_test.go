package devices

import "testing"

// The push-notifications tests that need a real GORM-compatible database
// live in repository_push_integration_test.go (build tag: integration)
// and are skipped under the default `go test ./...` run.
//
// The tests in this file are pure unit tests that only exercise the
// proto helper, so they always run.

func TestDeviceToProto_HandlesNilDomainID(t *testing.T) {
	d := Device{Token: "x"}
	got := DeviceToProto(d)
	if got.DomainId != 0 {
		t.Fatalf("DomainId = %d, want 0 for nil DomainID", got.DomainId)
	}
}

func TestDeviceToProto_PropagatesDomainID(t *testing.T) {
	did := uint(42)
	d := Device{Token: "x", DomainID: &did}
	got := DeviceToProto(d)
	if got.DomainId != 42 {
		t.Fatalf("DomainId = %d, want 42", got.DomainId)
	}
}

func TestDeviceToProto_PropagatesTimezone(t *testing.T) {
	d := Device{Token: "x", Timezone: "America/Mexico_City"}
	got := DeviceToProto(d)
	if got.Timezone != "America/Mexico_City" {
		t.Fatalf("Timezone = %q, want %q", got.Timezone, "America/Mexico_City")
	}
}

func TestDeviceToProto_EmptyTimezone(t *testing.T) {
	d := Device{Token: "x"}
	got := DeviceToProto(d)
	if got.Timezone != "" {
		t.Fatalf("Timezone = %q, want empty", got.Timezone)
	}
}
