package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAccessDoorResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewAccessDoorResource().(*accessDoorResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "door_id", "name", "floor_name", "extra_type", "is_open"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

func TestAccessDoorDataSourceSchema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewAccessDoorDataSource().(*accessDoorDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
}

func TestAccessDoorParams(t *testing.T) {
	r := &accessDoorResource{}

	// Unset floor_name → omitted; door_id always sent.
	m := &accessDoorResourceModel{
		DoorID:    types.StringValue("d1"),
		Name:      types.StringValue("Front"),
		FloorName: types.StringNull(),
		ExtraType: types.StringValue("standard"),
	}
	p := r.params(m)
	if p.DoorId == nil || *p.DoorId != "d1" {
		t.Errorf("DoorId = %v, want d1", p.DoorId)
	}
	if p.DoorName == nil || *p.DoorName != "Front" {
		t.Errorf("DoorName = %v, want Front", p.DoorName)
	}
	if p.FloorName != nil {
		t.Errorf("FloorName = %v, want nil (unset)", *p.FloorName)
	}
	body, _ := json.Marshal(p)
	if strings.Contains(string(body), "floor_name") {
		t.Errorf("unset floor_name must be omitted: %s", body)
	}

	// Configured floor_name is sent (moves the door under a floor).
	m.FloorName = types.StringValue("Floor 1")
	p = r.params(m)
	if p.FloorName == nil || *p.FloorName != "Floor 1" {
		t.Errorf("FloorName = %v, want Floor 1", p.FloorName)
	}
}

// TestAccessDoorDeleteIsNoOp asserts destroy does not touch the API (doors are
// discovered; destroy only removes the resource from state).
func TestAccessDoorDeleteIsNoOp(t *testing.T) {
	r := &accessDoorResource{} // nil client: a no-op Delete must not call it
	var resp resource.DeleteResponse
	r.Delete(context.Background(), resource.DeleteRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("Delete diagnostics = %v, want none", resp.Diagnostics)
	}
}
