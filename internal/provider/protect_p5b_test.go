package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

func enumPtr[T ~string](s string) *T { v := T(s); return &v }

func TestLightFromWire(t *testing.T) {
	l := &wire.Light{
		Id:   "l1",
		Mac:  "aabbccddeeff",
		Name: pStr("Porch"),
		LightModeSettings: wire.LightModeSettings{
			Mode:     enumPtr[wire.LightModeSettingsMode]("motion"),
			EnableAt: enumPtr[wire.LightModeSettingsEnableAt]("dark"),
		},
		LightDeviceSettings: wire.LightDeviceSettings{
			LedLevel:           pF64(3),
			IsIndicatorEnabled: pBool(true),
			PirSensitivity:     pF64(70),
			LuxSensitivity:     enumPtr[wire.LightDeviceSettingsLuxSensitivity]("high"),
		},
		LightOnSettings: wire.LightOnSettings{IsLedForceOn: pBool(false)},
		IsAdopted:       pBool(true),
		IsLightOn:       pBool(true),
		State:           enumPtr[wire.LightState]("CONNECTED"),
	}
	var m protectLightResourceModel
	m.MAC = types.StringNull() // import path
	m.fromLight(l)

	if m.ID.ValueString() != "l1" || m.MAC.ValueString() != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("id/mac = %q/%q", m.ID.ValueString(), m.MAC.ValueString())
	}
	if m.Name.ValueString() != "Porch" || m.Mode.ValueString() != "motion" || m.EnableAt.ValueString() != "dark" {
		t.Errorf("name/mode/enable_at = %q/%q/%q", m.Name.ValueString(), m.Mode.ValueString(), m.EnableAt.ValueString())
	}
	if m.LedLevel.ValueInt64() != 3 || m.LuxSensitivity.ValueString() != "high" || !m.IndicatorEnabled.ValueBool() {
		t.Errorf("device settings mismapped: led=%d lux=%q ind=%v", m.LedLevel.ValueInt64(), m.LuxSensitivity.ValueString(), m.IndicatorEnabled.ValueBool())
	}
	if m.State.ValueString() != "CONNECTED" || !m.IsLightOn.ValueBool() {
		t.Errorf("computed telemetry mismapped: state=%q on=%v", m.State.ValueString(), m.IsLightOn.ValueBool())
	}
}

func TestSensorThresholdMergeOmits(t *testing.T) {
	// All-unset group → nil (omitted from PATCH).
	if got := mergeThreshold(types.BoolNull(), types.Float64Null(), types.Float64Null(), types.Float64Null(), nil, nil, nil, nil); got != nil {
		t.Errorf("all-unset threshold = %+v, want nil", got)
	}
	// A fractional low threshold must survive (no truncation).
	got := mergeThreshold(types.BoolValue(true), types.Float64Value(20.5), types.Float64Null(), types.Float64Null(), nil, nil, nil, nil)
	if got == nil || got.LowThreshold == nil || *got.LowThreshold != 20.5 {
		t.Fatalf("threshold = %+v, want low=20.5", got)
	}
	if got.IsEnabled == nil || !*got.IsEnabled {
		t.Errorf("threshold is_enabled not carried")
	}
}

func TestSensorFromWirePreservesFraction(t *testing.T) {
	s := &wire.Sensor{
		Id:                  "s1",
		Mac:                 "112233445566",
		Name:                pStr("Garage"),
		MountType:           enumPtr[wire.SensorMountType]("garage"),
		TemperatureSettings: wire.TemperatureSensorSettings{IsEnabled: pBool(true), LowThreshold: pF64(18.5), HighThreshold: pF64(27.5)},
		MotionSettings:      wire.MotionSensorSettings{IsEnabled: pBool(true), Sensitivity: pF64(55)},
		State:               enumPtr[wire.SensorState]("CONNECTED"),
	}
	var m protectSensorResourceModel
	m.MAC = types.StringNull()
	m.fromSensor(s)
	if m.MountType.ValueString() != "garage" {
		t.Errorf("mount_type = %q", m.MountType.ValueString())
	}
	if m.TemperatureLow.ValueFloat64() != 18.5 || m.TemperatureHigh.ValueFloat64() != 27.5 {
		t.Errorf("temperature thresholds truncated: low=%v high=%v", m.TemperatureLow.ValueFloat64(), m.TemperatureHigh.ValueFloat64())
	}
	if m.MotionSensitivity.ValueFloat64() != 55 {
		t.Errorf("motion sensitivity = %v", m.MotionSensitivity.ValueFloat64())
	}
}

func TestChimeFromWireAndCameraIDs(t *testing.T) {
	c := &wire.Chime{
		Id:        "c1",
		Mac:       "aabbccddeeff",
		Name:      pStr("Hallway"),
		Volume:    pF64(60),
		CameraIds: []string{"camA", "camB"},
		IsAdopted: pBool(true),
		State:     enumPtr[wire.ChimeState]("CONNECTED"),
	}
	var m protectChimeResourceModel
	m.MAC = types.StringNull()
	m.fromChime(c)
	if m.Volume.ValueInt64() != 60 {
		t.Errorf("volume = %d", m.Volume.ValueInt64())
	}
	var ids []string
	m.CameraIDs.ElementsAs(context.Background(), &ids, false)
	if len(ids) != 2 {
		t.Errorf("camera_ids = %v, want 2", ids)
	}
}

func TestViewerFromWire(t *testing.T) {
	v := &wire.Viewer{
		Id:          "v1",
		Mac:         "aabbccddeeff",
		Name:        pStr("Lobby TV"),
		Liveview:    pStr("lv9"),
		StreamLimit: pF64(4),
		State:       enumPtr[wire.ViewerState]("CONNECTED"),
	}
	var m protectViewerResourceModel
	m.MAC = types.StringNull()
	m.fromViewer(v)
	if m.Liveview.ValueString() != "lv9" || m.StreamLimit.ValueInt64() != 4 {
		t.Errorf("liveview/stream_limit = %q/%d", m.Liveview.ValueString(), m.StreamLimit.ValueInt64())
	}
}

func TestLiveviewSlotsRoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	// Build a wire slots payload, parse to state, feed back through config(), and
	// confirm the raw payload round-trips (cameras + cycle settings preserved).
	raw := []interface{}{
		map[string]interface{}{"cameras": []interface{}{"camA", "camB"}, "cycleMode": "motion", "cycleInterval": float64(8)},
		map[string]interface{}{"cameras": []interface{}{"camC"}},
	}
	list := slotsToState(ctx, &raw, &diags)
	if diags.HasError() {
		t.Fatalf("slotsToState: %v", diags)
	}
	if list.IsNull() || len(list.Elements()) != 2 {
		t.Fatalf("parsed slots = %d, want 2", len(list.Elements()))
	}

	r := &protectLiveviewResource{}
	plan := &protectLiveviewResourceModel{Name: types.StringValue("Grid"), Slots: list}
	cfg := r.config(ctx, plan, &diags)
	if diags.HasError() {
		t.Fatalf("config: %v", diags)
	}
	body, _ := json.Marshal(cfg)
	s := string(body)
	if !strings.Contains(s, `"cameras":["camA","camB"]`) || !strings.Contains(s, `"cycleMode":"motion"`) || !strings.Contains(s, `"cycleInterval":8`) {
		t.Errorf("slot round-trip lost data: %s", s)
	}
	if !strings.Contains(s, `"cameras":["camC"]`) {
		t.Errorf("second slot lost: %s", s)
	}
}

func TestLiveviewConfigOmitsSlotsWhenUnset(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	r := &protectLiveviewResource{}
	plan := &protectLiveviewResourceModel{
		Name:  types.StringValue("Grid"),
		Slots: types.ListNull(types.ObjectType{AttrTypes: liveviewSlotAttrTypes}),
	}
	cfg := r.config(ctx, plan, &diags)
	body, _ := json.Marshal(cfg)
	if strings.Contains(string(body), "slots") {
		t.Errorf("unset slots must be omitted: %s", body)
	}
	if !strings.Contains(string(body), `"name":"Grid"`) {
		t.Errorf("name missing: %s", body)
	}
}

// TestP5bResourceSchemas builds each new resource's schema and asserts it is
// valid (no diagnostics).
func TestP5bResourceSchemas(t *testing.T) {
	for name, res := range map[string]resource.Resource{
		"light":    NewProtectLightResource(),
		"sensor":   NewProtectSensorResource(),
		"chime":    NewProtectChimeResource(),
		"viewer":   NewProtectViewerResource(),
		"liveview": NewProtectLiveviewResource(),
	} {
		var resp resource.SchemaResponse
		res.(interface {
			Schema(context.Context, resource.SchemaRequest, *resource.SchemaResponse)
		}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%s schema invalid: %v", name, resp.Diagnostics)
		}
	}
}
