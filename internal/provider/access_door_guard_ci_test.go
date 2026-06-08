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

func TestAccessDoorGuardResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewAccessDoorGuardResource().(*accessDoorGuardResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "door_id", "user_ids"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

func TestAccessDoorGuardDataSourceSchema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewAccessDoorGuardDataSource().(*accessDoorGuardDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
}

func TestAccessDoorGuardParams(t *testing.T) {
	r := &accessDoorGuardResource{}
	ctx := context.Background()

	// Configured user_ids → sent; door_id always sent.
	m := &accessDoorGuardResourceModel{
		DoorID:  types.StringValue("d1"),
		UserIDs: sliceToStringSet([]string{"u1", "u2"}),
	}
	var diags diag.Diagnostics
	p := r.params(ctx, m, &diags)
	if diags.HasError() {
		t.Fatalf("params diagnostics: %v", diags)
	}
	if p.DoorId == nil || *p.DoorId != "d1" {
		t.Errorf("DoorId = %v, want d1", p.DoorId)
	}
	if p.UserIds == nil || len(*p.UserIds) != 2 {
		t.Fatalf("UserIds = %v, want 2 entries", p.UserIds)
	}

	// Explicit empty set clears the guard set: it must marshal as user_ids:[]
	// (a non-nil empty slice), not be omitted.
	m.UserIDs = sliceToStringSet([]string{})
	p = r.params(ctx, m, &diags)
	if p.UserIds == nil {
		t.Fatalf("empty user_ids must send a non-nil slice (clears the set)")
	}
	body, _ := json.Marshal(p)
	if !strings.Contains(string(body), `"user_ids":[]`) {
		t.Errorf("empty user_ids must marshal to user_ids:[]: %s", body)
	}

	// Null (unconfigured) user_ids → omitted (leaves the guard set untouched).
	m.UserIDs = types.SetNull(types.StringType)
	p = r.params(ctx, m, &diags)
	if p.UserIds != nil {
		t.Errorf("UserIds = %v, want nil (unset)", *p.UserIds)
	}
	body, _ = json.Marshal(p)
	if strings.Contains(string(body), "user_ids") {
		t.Errorf("unset user_ids must be omitted: %s", body)
	}
}

// TestAccessDoorGuardDeleteIsNoOp asserts destroy does not touch the API (the
// guard set belongs to discovered hardware; destroy only removes state).
func TestAccessDoorGuardDeleteIsNoOp(t *testing.T) {
	r := &accessDoorGuardResource{} // nil client: a no-op Delete must not call it
	var resp resource.DeleteResponse
	r.Delete(context.Background(), resource.DeleteRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("Delete diagnostics = %v, want none", resp.Diagnostics)
	}
}
