package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access"
)

// TestIsNotFoundErrIncludesAccess guards the remove-on-404 contract: a deleted
// Access object surfaces access.ErrNotFound, which Read maps to remove-from-state
// via isNotFoundErr. A regression here would make out-of-band deletes hard-error
// on the next plan instead of reconciling (invariant 6).
func TestIsNotFoundErrIncludesAccess(t *testing.T) {
	if !isNotFoundErr(access.ErrNotFound) {
		t.Error("isNotFoundErr(access.ErrNotFound) = false, want true")
	}
}

// TestAccessDoorGroupResourceSchema builds the (currently un-registered) managed
// resource's schema and asserts it is valid and carries the expected attributes,
// so the code that ships ready-to-register stays correct.
func TestAccessDoorGroupResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewAccessDoorGroupResource().(*accessDoorGroupResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "name", "group_type", "door_ids"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

// TestDoorGroupParamsOmitsUnsetDoorIDs asserts an unset (null) door_ids leaves
// DoorIds nil on the write DTO — so membership is untouched rather than cleared.
func TestDoorGroupParamsOmitsUnsetDoorIDs(t *testing.T) {
	r := &accessDoorGroupResource{}
	var diags diag.Diagnostics
	m := &accessDoorGroupResourceModel{
		Name:    types.StringValue("Lobby"),
		DoorIDs: types.SetNull(types.StringType),
	}
	p := r.params(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("params: %v", diags)
	}
	if p.DoorIds != nil {
		t.Errorf("DoorIds = %v, want nil for unset door_ids", *p.DoorIds)
	}
	if p.Name == nil || *p.Name != "Lobby" {
		t.Errorf("Name = %v, want Lobby", p.Name)
	}
	// group_type unset → omitted entirely.
	body, _ := json.Marshal(p)
	if strings.Contains(string(body), "group_type") {
		t.Errorf("unset group_type must be omitted: %s", body)
	}
	if strings.Contains(string(body), "door_ids") {
		t.Errorf("unset door_ids must be omitted: %s", body)
	}
}

// TestDoorGroupParamsSendsConfigured asserts configured fields land on the DTO,
// including an explicitly-empty door_ids ([]), which clears membership.
func TestDoorGroupParamsSendsConfigured(t *testing.T) {
	r := &accessDoorGroupResource{}
	var diags diag.Diagnostics
	m := &accessDoorGroupResourceModel{
		Name:      types.StringValue("Garage"),
		GroupType: types.StringValue("door"),
		DoorIDs:   sliceToStringSet([]string{"d1", "d2"}),
	}
	p := r.params(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("params: %v", diags)
	}
	if p.DoorIds == nil || len(*p.DoorIds) != 2 {
		t.Fatalf("DoorIds = %v, want [d1 d2]", p.DoorIds)
	}
	if p.GroupType == nil || *p.GroupType != "door" {
		t.Errorf("GroupType = %v, want door", p.GroupType)
	}
	body, _ := json.Marshal(p)
	if !strings.Contains(string(body), `"name":"Garage"`) || !strings.Contains(string(body), `"door_ids":["d1","d2"]`) {
		t.Errorf("body missing configured fields: %s", body)
	}

	// Explicit empty set is sent as [] (clear membership), not omitted.
	m.DoorIDs = sliceToStringSet([]string{})
	p = r.params(context.Background(), m, &diags)
	if p.DoorIds == nil || len(*p.DoorIds) != 0 {
		t.Errorf("DoorIds = %v, want explicit empty slice", p.DoorIds)
	}
}

func TestInt64PtrToState(t *testing.T) {
	if got := int64PtrToState(nil); !got.IsNull() {
		t.Errorf("int64PtrToState(nil) = %v, want null", got)
	}
	v := int64(7)
	if got := int64PtrToState(&v); got.ValueInt64() != 7 {
		t.Errorf("int64PtrToState(7) = %v, want 7", got)
	}
}
