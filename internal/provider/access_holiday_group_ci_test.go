package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
)

func TestAccessHolidayGroupResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewAccessHolidayGroupResource().(*accessHolidayGroupResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "name", "description", "is_private", "template_name", "holiday_ids", "holiday_count"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

func TestAccessHolidayGroupDataSourceSchema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewAccessHolidayGroupDataSource().(*accessHolidayGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
}

func TestAccessHolidayGroupParamsOmitsUnset(t *testing.T) {
	r := &accessHolidayGroupResource{}
	var diags diag.Diagnostics
	m := &accessHolidayGroupResourceModel{
		Name:       types.StringValue("US Holidays"),
		HolidayIDs: types.SetNull(types.StringType),
	}
	p := r.params(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("params: %v", diags)
	}
	if p.Name == nil || *p.Name != "US Holidays" {
		t.Errorf("Name = %v, want US Holidays", p.Name)
	}
	if p.HolidayIds != nil {
		t.Errorf("HolidayIds = %v, want nil for unset", *p.HolidayIds)
	}
	body, _ := json.Marshal(p)
	for _, k := range []string{"holiday_ids", "description", "is_private", "template_name"} {
		if strings.Contains(string(body), k) {
			t.Errorf("unset %q must be omitted: %s", k, body)
		}
	}
}

func TestAccessHolidayGroupParamsSendsConfigured(t *testing.T) {
	r := &accessHolidayGroupResource{}
	var diags diag.Diagnostics
	m := &accessHolidayGroupResourceModel{
		Name:       types.StringValue("US Holidays"),
		IsPrivate:  types.BoolValue(true),
		HolidayIDs: sliceToStringSet([]string{"h1", "h2"}),
	}
	p := r.params(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("params: %v", diags)
	}
	if p.IsPrivate == nil || !*p.IsPrivate {
		t.Errorf("IsPrivate = %v, want true", p.IsPrivate)
	}
	if p.HolidayIds == nil || len(*p.HolidayIds) != 2 {
		t.Fatalf("HolidayIds = %v, want [h1 h2]", p.HolidayIds)
	}
	body, _ := json.Marshal(p)
	if !strings.Contains(string(body), `"holiday_ids":["h1","h2"]`) {
		t.Errorf("body missing holiday_ids: %s", body)
	}
}

// TestHolidayMemberIDs covers the unique_id-then-id fallback and the nil case.
func TestHolidayMemberIDs(t *testing.T) {
	if ids := holidayMemberIDs(nil); len(ids) != 0 {
		t.Errorf("nil holidays = %v, want empty", ids)
	}
	holidays := []wire.HolidayBoHolidayParams{
		{UniqueId: ptrStr("h1")},
		{Id: ptrStr("h2")}, // unique_id empty → falls back to id
		{},                 // both empty → skipped
	}
	ids := holidayMemberIDs(&holidays)
	if len(ids) != 2 || ids[0] != "h1" || ids[1] != "h2" {
		t.Errorf("ids = %v, want [h1 h2]", ids)
	}
}

func ptrStr(s string) *string { return &s }
