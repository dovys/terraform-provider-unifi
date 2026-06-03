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
)

func TestAccessPolicyResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewAccessPolicyResource().(*accessPolicyResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "name", "schedule_id", "enabled", "resources", "user_count", "door_count"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

func TestAccessPolicyDataSourceSchema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewAccessPolicyDataSource().(*accessPolicyDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
}

// TestAccessPolicyParamsOmitsUnset asserts unset optional fields leave the write
// DTO's pointers nil (omitted), so a refresh doesn't clobber controller state.
func TestAccessPolicyParamsOmitsUnset(t *testing.T) {
	r := &accessPolicyResource{}
	var diags diag.Diagnostics
	m := &accessPolicyResourceModel{
		Name:       types.StringValue("Staff"),
		ScheduleID: types.StringNull(),
		Enabled:    types.BoolNull(),
		Resources:  types.SetNull(accessPolicyResourceSetType.ElemType),
	}
	p := r.params(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("params: %v", diags)
	}
	if p.Name == nil || *p.Name != "Staff" {
		t.Errorf("Name = %v, want Staff", p.Name)
	}
	if p.ScheduleId != nil || p.Configs != nil || p.Resource != nil {
		t.Errorf("unset fields must stay nil: schedule=%v configs=%v resource=%v", p.ScheduleId, p.Configs, p.Resource)
	}
	body, _ := json.Marshal(p)
	for _, k := range []string{"schedule_id", "configs", "resource"} {
		if strings.Contains(string(body), k) {
			t.Errorf("unset %q must be omitted: %s", k, body)
		}
	}
}

// TestAccessPolicyParamsSendsConfigured asserts configured fields — including the
// enabled→is_active mapping and the {type,id} resource grants — land on the DTO.
func TestAccessPolicyParamsSendsConfigured(t *testing.T) {
	r := &accessPolicyResource{}
	var diags diag.Diagnostics
	resSet, d := types.SetValueFrom(context.Background(), accessPolicyResourceSetType.ElemType, []accessPolicyResourceObjectModel{
		{Type: types.StringValue("door"), ID: types.StringValue("d1")},
		{Type: types.StringValue("door_group"), ID: types.StringValue("g1")},
	})
	diags.Append(d...)
	m := &accessPolicyResourceModel{
		Name:       types.StringValue("Staff"),
		ScheduleID: types.StringValue("sch1"),
		Enabled:    types.BoolValue(true),
		Resources:  resSet,
	}
	p := r.params(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("params: %v", diags)
	}
	if p.ScheduleId == nil || *p.ScheduleId != "sch1" {
		t.Errorf("ScheduleId = %v, want sch1", p.ScheduleId)
	}
	if p.Configs == nil || p.Configs.IsActive == nil || !*p.Configs.IsActive {
		t.Errorf("Configs.IsActive = %v, want true", p.Configs)
	}
	if p.Resource == nil || len(*p.Resource) != 2 {
		t.Fatalf("Resource = %v, want 2 grants", p.Resource)
	}
	body, _ := json.Marshal(p)
	if !strings.Contains(string(body), `"type":"door"`) || !strings.Contains(string(body), `"id":"d1"`) {
		t.Errorf("body missing resource grant: %s", body)
	}
}
