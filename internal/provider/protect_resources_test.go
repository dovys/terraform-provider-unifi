package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

func pStr(s string) *string   { return &s }
func pBool(b bool) *bool      { return &b }
func pF64(f float64) *float64 { return &f }

func TestMACCanonical(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"28:70:4E:11:22:33", "28:70:4e:11:22:33"},
		{"28704e112233", "28:70:4e:11:22:33"},
		{"28-70-4e-11-22-33", "28:70:4e:11:22:33"},
		{"2870.4e11.2233", "28:70:4e:11:22:33"},
		{"  28:70:4e:11:22:33 ", "28:70:4e:11:22:33"},
	} {
		if got := macCanonical(tc.in); got != tc.want {
			t.Errorf("macCanonical(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMergeHelpersPlanWins(t *testing.T) {
	if got := mergeStr(types.StringValue("plan"), pStr("cur")); got == nil || *got != "plan" {
		t.Errorf("mergeStr set: got %v, want plan", got)
	}
	if got := mergeStr(types.StringNull(), pStr("cur")); got == nil || *got != "cur" {
		t.Errorf("mergeStr null: got %v, want cur", got)
	}
	if got := mergeStr(types.StringNull(), nil); got != nil {
		t.Errorf("mergeStr null+nil: got %v, want nil", got)
	}
	if got := mergeBool(types.BoolValue(true), pBool(false)); got == nil || *got != true {
		t.Errorf("mergeBool set: got %v, want true", got)
	}
	if got := mergeBool(types.BoolNull(), pBool(true)); got == nil || *got != true {
		t.Errorf("mergeBool null: got %v, want cur(true)", got)
	}
	if got := mergeFloat(types.Int64Value(80), pF64(10)); got == nil || *got != 80 {
		t.Errorf("mergeFloat set: got %v, want 80", got)
	}
	if got := mergeFloat(types.Int64Null(), pF64(10)); got == nil || *got != 10 {
		t.Errorf("mergeFloat null: got %v, want cur(10)", got)
	}
}

func TestIfaceStrings(t *testing.T) {
	in := []interface{}{"person", "vehicle", 42, nil, "package"}
	got := ifaceStrings(&in)
	want := []string{"person", "vehicle", "package"}
	if len(got) != len(want) {
		t.Fatalf("ifaceStrings = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ifaceStrings[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if ifaceStrings(nil) != nil {
		t.Error("ifaceStrings(nil) should be nil")
	}
}

// TestCameraConfigOmitsUnsetFields proves the owned overlay only serializes the
// fields the provider sets — controller-managed state is never echoed back from
// a full read object (no omitempty would clobber it).
func TestCameraConfigOmitsUnsetFields(t *testing.T) {
	cfg := cameraConfig{
		Name:        pStr("Front Door"),
		LedSettings: &ledConfig{IsEnabled: pBool(true)},
	}
	b, _ := json.Marshal(cfg)
	s := string(b)
	if !strings.Contains(s, `"name":"Front Door"`) {
		t.Errorf("missing name: %s", s)
	}
	if !strings.Contains(s, `"ledSettings":{"isEnabled":true}`) {
		t.Errorf("missing ledSettings: %s", s)
	}
	for _, absent := range []string{"micVolume", "hdrType", "videoMode", "osdSettings", "recordingSettings", "smartDetectSettings", "lcdMessage", "isMicEnabled"} {
		if strings.Contains(s, absent) {
			t.Errorf("unset field %q must be omitted: %s", absent, s)
		}
	}
}

func TestCameraFromWire(t *testing.T) {
	objs := []interface{}{"person", "vehicle"}
	cam := &wire.Camera{
		Id:           "c1",
		Mac:          "aabbccddeeff",
		Name:         pStr("Driveway"),
		IsMicEnabled: pBool(true),
		MicVolume:    pF64(75),
		HdrType:      pStr("auto"),
		VideoMode:    (*wire.CameraVideoMode)(pStr("default")),
		IsAdopted:    pBool(true),
		HasSpeaker:   pBool(true),
		Host:         pStr("192.0.2.5"),
		State:        (*wire.CameraState)(pStr("CONNECTED")),
		Type:         pStr("UVC G4 Doorbell"),
		LedSettings:  wire.LedSettings{IsEnabled: pBool(false)},
		OsdSettings:  wire.OsdSettings{IsNameEnabled: pBool(true), IsDateEnabled: pBool(false)},
		RecordingSettings: wire.CameraRecordingSettings{
			Mode: (*wire.CameraRecordingSettingsMode)(pStr("detections")),
		},
		SmartDetectSettings: wire.SmartDetectSettings{ObjectTypes: &objs},
	}
	var m protectCameraResourceModel
	m.MAC = types.StringValue("aabbccddeeff")
	var diags diag.Diagnostics
	m.fromCamera(context.Background(), cam, &diags)
	if diags.HasError() {
		t.Fatalf("fromCamera diags: %v", diags)
	}
	if m.ID.ValueString() != "c1" {
		t.Errorf("id = %q, want c1", m.ID.ValueString())
	}
	if m.Name.ValueString() != "Driveway" {
		t.Errorf("name = %q", m.Name.ValueString())
	}
	if m.MicVolume.ValueInt64() != 75 {
		t.Errorf("mic_volume = %d, want 75", m.MicVolume.ValueInt64())
	}
	if m.StatusLightEnabled.ValueBool() != false {
		t.Errorf("status_light_enabled = %v, want false", m.StatusLightEnabled.ValueBool())
	}
	if m.RecordingMode.ValueString() != "detections" {
		t.Errorf("recording_mode = %q, want detections", m.RecordingMode.ValueString())
	}
	if m.State.ValueString() != "CONNECTED" {
		t.Errorf("state = %q, want CONNECTED", m.State.ValueString())
	}
	var objsGot []string
	m.SmartDetectObjectTypes.ElementsAs(context.Background(), &objsGot, false)
	if len(objsGot) != 2 {
		t.Errorf("smart_detect_object_types = %v, want 2 entries", objsGot)
	}
}

// TestNVRUpdateOmitsUnsetFields proves the generated wire.NvrUpdate write DTO
// is omitempty-tagged, so a partial PATCH carries only the set fields.
func TestNVRUpdateOmitsUnsetFields(t *testing.T) {
	cfg := wire.NvrUpdate{Name: pStr("Console"), IsSshEnabled: pBool(true)}
	b, _ := json.Marshal(cfg)
	s := string(b)
	if !strings.Contains(s, `"name":"Console"`) || !strings.Contains(s, `"isSshEnabled":true`) {
		t.Errorf("expected name+isSshEnabled: %s", s)
	}
	for _, absent := range []string{"timezone", "timeFormat", "temperatureUnit", "recordingRetentionDurationMs", "disableAudio"} {
		if strings.Contains(s, absent) {
			t.Errorf("unset field %q must be omitted: %s", absent, s)
		}
	}
}

func TestNVRFromWire(t *testing.T) {
	n := &wire.Nvr{
		Id:                           "nvr1",
		Name:                         pStr("HQ"),
		Timezone:                     pStr("UTC"),
		TimeFormat:                   (*wire.NvrTimeFormat)(pStr("24h")),
		TemperatureUnit:              (*wire.NvrTemperatureUnit)(pStr("C")),
		IsSshEnabled:                 pBool(true),
		AutoRetentionEnabled:         true,
		RecordingRetentionDurationMs: 86400000,
		Version:                      pStr("7.1.69"),
	}
	var m protectNVRSettingsModel
	m.fromNVR(n)
	if m.ID.ValueString() != "nvr1" || m.Version.ValueString() != "7.1.69" {
		t.Errorf("id/version = %q/%q", m.ID.ValueString(), m.Version.ValueString())
	}
	if m.TimeFormat.ValueString() != "24h" || m.TemperatureUnit.ValueString() != "C" {
		t.Errorf("timeFormat/tempUnit = %q/%q", m.TimeFormat.ValueString(), m.TemperatureUnit.ValueString())
	}
	if !m.AutoRetentionEnabled.ValueBool() {
		t.Error("auto_retention_enabled should be true")
	}
	if m.RecordingRetentionMs.ValueInt64() != 86400000 {
		t.Errorf("retention = %d", m.RecordingRetentionMs.ValueInt64())
	}
}
