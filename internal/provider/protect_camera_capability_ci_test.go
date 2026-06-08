package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

func TestCameraCapabilityViolations(t *testing.T) {
	b := func(v bool) *bool { return &v }
	full := wire.FeatureFlags{
		HasHdr: b(true), HasMic: b(true), HasLedStatus: b(true),
		HasSmartDetect: b(true), HasLcdScreen: b(true),
	}
	none := wire.FeatureFlags{} // all nil → unsupported

	// All gated fields configured (unset fields are the zero/null value).
	cfgAll := &protectCameraResourceModel{
		HDRType:                types.StringValue("auto"),
		MicEnabled:             types.BoolValue(true),
		StatusLightEnabled:     types.BoolValue(true),
		SmartDetectObjectTypes: sliceToStringSet([]string{"person"}),
		LCDMessageType:         types.StringValue("DO_NOT_DISTURB"),
	}

	if !cameraConfiguresGatedField(cfgAll) {
		t.Fatal("cameraConfiguresGatedField(cfgAll) = false, want true")
	}
	if v := cameraCapabilityViolations(cfgAll, full); len(v) != 0 {
		t.Errorf("capable camera: got violations %v, want none", v)
	}
	if v := cameraCapabilityViolations(cfgAll, none); len(v) != 5 {
		t.Errorf("incapable camera: got %d violations, want 5: %v", len(v), v)
	}

	// Only the microphone is missing → exactly one mic_enabled violation.
	micMissing := wire.FeatureFlags{HasHdr: b(true), HasLedStatus: b(true), HasSmartDetect: b(true), HasLcdScreen: b(true)}
	if v := cameraCapabilityViolations(cfgAll, micMissing); len(v) != 1 || v[0].Attr != "mic_enabled" {
		t.Fatalf("mic-missing camera: got %v, want one mic_enabled violation", v)
	}

	// No gated fields configured → never any violations (and no read needed).
	nameOnly := &protectCameraResourceModel{Name: types.StringValue("Front Door")}
	if cameraConfiguresGatedField(nameOnly) {
		t.Error("cameraConfiguresGatedField(nameOnly) = true, want false")
	}
	if v := cameraCapabilityViolations(nameOnly, none); len(v) != 0 {
		t.Errorf("no gated fields: got %v, want none", v)
	}
}
