package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// The v2 API rejects anything but the uppercase enum, so lowercase must fail at
// plan time.
func TestWANSLAMonitorTypeValidator(t *testing.T) {
	var resp resource.SchemaResponse
	NewWANSLAResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	mons, ok := resp.Schema.Attributes["monitors"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("monitors is not a ListNestedAttribute")
	}
	typ := mons.NestedObject.Attributes["type"]
	for _, good := range []string{"ICMP", "DNS", "SPEED_LIMIT"} {
		assertStringEnum(t, typ, "type", good, "icmp")
	}
	for _, bad := range []string{"dns", "speedLimit", "Icmp"} {
		assertStringEnum(t, typ, "type", "ICMP", bad)
	}
}

func TestWANSLAToWireSendsControllerCasing(t *testing.T) {
	m := wanSLAResourceModel{
		Name: types.StringValue("primary"),
		Monitors: []wanSLAMonitorModel{{
			Target:     types.StringValue("192.0.2.1"),
			Type:       types.StringValue("ICMP"),
			Interval:   types.Int64Value(60),
			TimePeriod: types.Int64Value(300),
		}},
	}
	b, err := json.Marshal(m.toWire())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"type":"ICMP"`) {
		t.Errorf("body %s lacks \"type\":\"ICMP\"", b)
	}
}

// Read must round-trip the controller's casing so an imported SLA plans clean.
func TestWANSLARefreshKeepsCasing(t *testing.T) {
	doc := `{"_id":"s1","name":"primary","monitor_health_type":"ALL",
		"monitors":[{"alert":{},"interval":60,"monitor_id":1,"target":"192.0.2.1","time_period":300,"type":"DNS","domain":"example.com"}]}`
	var sla wire.WanSlas
	if err := json.Unmarshal([]byte(doc), &sla); err != nil {
		t.Fatal(err)
	}
	m := wireToWANSLAModel(&sla)
	if got := m.Monitors[0].Type.ValueString(); got != "DNS" {
		t.Errorf("type = %q, want DNS", got)
	}
	if got := m.MonitorHealthType.ValueString(); got != "ALL" {
		t.Errorf("monitor_health_type = %q, want ALL", got)
	}
	if got := m.Monitors[0].TimePeriod.ValueInt64(); got != 300 {
		t.Errorf("time_period = %d, want 300", got)
	}
}
