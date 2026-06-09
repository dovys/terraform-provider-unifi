package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

func TestMdnsResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewMdnsResource().(*mdnsResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "mode", "network_ids"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
	if !resp.Schema.Attributes["mode"].IsRequired() {
		t.Error("mode must be Required")
	}
}

// TestMdnsOverlayScope is the model→wire serialization: the overlay sets the two
// mDNS fields and leaves the document's required IGMP fields untouched, and the
// id list is sent only for mode "some" (an explicit [] otherwise).
func TestMdnsOverlayScope(t *testing.T) {
	ctx := context.Background()

	// A document as Get would return it: required IGMP fields populated.
	baseDoc := func() *wire.Global {
		return &wire.Global{
			IgmpSnoopingFor:           wire.GlobalIgmpSnoopingForAll,
			IgmpSnoopingForNetworkIds: []string{"net-igmp"},
			MdnsEnabledFor:            wire.GlobalIgmpSnoopingForNone,
			MdnsEnabledForNetworkIds:  []string{"stale"},
		}
	}

	cases := []struct {
		name      string
		model     *mdnsModel
		wantMode  string
		wantIDs   []string
		wantSends []string
	}{
		{
			name:      "some with ids",
			model:     &mdnsModel{Mode: types.StringValue("some"), NetworkIDs: sliceToStringSet([]string{"a", "b"})},
			wantMode:  "some",
			wantIDs:   []string{"a", "b"},
			wantSends: []string{`"mdns_enabled_for":"some"`, `"mdns_enabled_for_network_ids":["a","b"]`, `"igmp_snooping_for":"all"`},
		},
		{
			name:      "all forces empty ids",
			model:     &mdnsModel{Mode: types.StringValue("all"), NetworkIDs: sliceToStringSet([]string{"a", "b"})},
			wantMode:  "all",
			wantIDs:   []string{},
			wantSends: []string{`"mdns_enabled_for":"all"`, `"mdns_enabled_for_network_ids":[]`},
		},
		{
			name:      "none with null ids sends empty list",
			model:     &mdnsModel{Mode: types.StringValue("none"), NetworkIDs: types.SetNull(types.StringType)},
			wantMode:  "none",
			wantIDs:   []string{},
			wantSends: []string{`"mdns_enabled_for":"none"`, `"mdns_enabled_for_network_ids":[]`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d diag.Diagnostics
			doc := baseDoc()
			overlayMdnsScope(ctx, tc.model, doc, &d)
			if d.HasError() {
				t.Fatalf("overlay: %v", d)
			}
			if string(doc.MdnsEnabledFor) != tc.wantMode {
				t.Errorf("MdnsEnabledFor = %q, want %q", doc.MdnsEnabledFor, tc.wantMode)
			}
			if len(doc.MdnsEnabledForNetworkIds) != len(tc.wantIDs) {
				t.Errorf("ids = %v, want %v", doc.MdnsEnabledForNetworkIds, tc.wantIDs)
			}
			// The required IGMP fields must survive (replace-style PUT).
			if doc.IgmpSnoopingFor != wire.GlobalIgmpSnoopingForAll || len(doc.IgmpSnoopingForNetworkIds) != 1 {
				t.Errorf("IGMP fields clobbered: %+v", doc)
			}
			b, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			for _, want := range tc.wantSends {
				if !strings.Contains(string(b), want) {
					t.Errorf("body %s missing %s", b, want)
				}
			}
		})
	}
}

// TestMdnsApplyDocToModel is the wire→model direction: mode round-trips, and
// network_ids is null for non-"some" modes so an imported scope doesn't churn
// against an omitted config.
func TestMdnsApplyDocToModel(t *testing.T) {
	cases := []struct {
		name        string
		doc         *wire.Global
		wantMode    string
		wantNullIDs bool
		wantIDs     []string
	}{
		{
			name:     "some keeps ids",
			doc:      &wire.Global{MdnsEnabledFor: wire.GlobalIgmpSnoopingForSome, MdnsEnabledForNetworkIds: []string{"a", "b"}},
			wantMode: "some",
			wantIDs:  []string{"a", "b"},
		},
		{
			name:        "all nulls ids",
			doc:         &wire.Global{MdnsEnabledFor: wire.GlobalIgmpSnoopingForAll, MdnsEnabledForNetworkIds: []string{"ignored"}},
			wantMode:    "all",
			wantNullIDs: true,
		},
		{
			name:        "none nulls ids",
			doc:         &wire.Global{MdnsEnabledFor: wire.GlobalIgmpSnoopingForNone, MdnsEnabledForNetworkIds: []string{}},
			wantMode:    "none",
			wantNullIDs: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m mdnsModel
			applyMdnsDocToModel(tc.doc, &m)
			if m.ID.ValueString() != mdnsID {
				t.Errorf("id = %q, want %q", m.ID.ValueString(), mdnsID)
			}
			if m.Mode.ValueString() != tc.wantMode {
				t.Errorf("mode = %q, want %q", m.Mode.ValueString(), tc.wantMode)
			}
			if tc.wantNullIDs {
				if !m.NetworkIDs.IsNull() {
					t.Errorf("network_ids = %v, want null for mode %q", m.NetworkIDs, tc.wantMode)
				}
				return
			}
			var got []string
			m.NetworkIDs.ElementsAs(context.Background(), &got, false)
			if len(got) != len(tc.wantIDs) {
				t.Errorf("network_ids = %v, want %v", got, tc.wantIDs)
			}
		})
	}
}

// TestMdnsValidateConfig: network_ids alongside a non-"some" mode is a config
// error; "some" (or an empty list) is accepted.
func TestMdnsValidateConfig(t *testing.T) {
	cases := []struct {
		name    string
		mode    types.String
		ids     types.Set
		wantErr bool
	}{
		{"some with ids ok", types.StringValue("some"), sliceToStringSet([]string{"a"}), false},
		{"all with ids errors", types.StringValue("all"), sliceToStringSet([]string{"a"}), true},
		{"none with ids errors", types.StringValue("none"), sliceToStringSet([]string{"a"}), true},
		{"all with null ids ok", types.StringValue("all"), types.SetNull(types.StringType), false},
		{"some with empty ids ok", types.StringValue("some"), sliceToStringSet([]string{}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d diag.Diagnostics
			cfg := &mdnsModel{Mode: tc.mode, NetworkIDs: tc.ids}
			validateMdnsConfig(cfg, &d)
			if got := d.HasError(); got != tc.wantErr {
				t.Errorf("hasError = %v, want %v (%v)", got, tc.wantErr, d)
			}
		})
	}
}
