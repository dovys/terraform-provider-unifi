//go:build hil

// Live-controller regression for issue #89, verified against UniFi Network
// 10.4.57. It pins the controller's ACTUAL DHCP-guard contract so a future spec
// bump or refactor can't silently reintroduce the bug:
//
//	A (NOT real): a plain non-subnet update via a sparse PUT — {_id, name} — is
//	  ACCEPTED. The controller shallow-merges networkconf and does not require
//	  ip_subnet on a plain update. (So PR #92's "restore GET-merge-PUT" premise is
//	  false.)
//	B (real): enabling dhcpguard with no trusted DHCP server is REJECTED with
//	  api.err.MissingIPAddress — and the missing field is NOT ip_subnet (a full
//	  doc carrying ip_subnet still fails). The trusted server lives in
//	  dhcpd_ip_1..3; supplying one makes the write succeed.
//
// Additive + inert + self-tearing-down: it creates a brand-new tfacc-hil-
// network on a DOCUMENTATION subnet (RFC 5737 192.0.2.0/24 — no client routes to
// it, no lease handed out) with the DHCP server OFF, and deletes it in a
// t.Cleanup. It never mutates an operator object. Gated like the W* write tiers.
//
//	UNIFI_HIL_ALLOW_WRITES=1 UNIFI_HIL_WRITE_HOST=$UNIFI_HOST \
//	  go test -tags hil ./test/hil -run TestHILVerifyDHCPGuard -v
package hil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/acctest"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// probeSubnet is a documentation prefix (RFC 5737). A network on it is inert:
// no real client routes to it, so guard/DHCP toggles have no live effect.
const probeSubnet = "192.0.2.1/24"

func ptr[T any](v T) *T { return &v }

// isMissingIPAddress reports whether err is the controller's guard-needs-a-server
// rejection (api.err.MissingIPAddress), surfaced through the APIError message.
func isMissingIPAddress(err error) bool {
	return err != nil && strings.Contains(err.Error(), "MissingIPAddress")
}

func TestHILVerifyDHCPGuard(t *testing.T) {
	hilWriteGate(t, "verify")

	c, err := acctest.Client()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ctx := context.Background()
	nets := c.Network().Networks()

	// Create a baseline network: subnet present, DHCP server off, guard off.
	base := &wire.Networkconf{
		Name:         ptr(tfaccHILPrefix + "dhcpguard"),
		Purpose:      ptr(wire.NetworkconfPurpose("corporate")),
		Networkgroup: ptr("LAN"),
		IpSubnet:     ptr(probeSubnet),
		VlanEnabled:  ptr(true),
		Vlan:         ptr(4000), // valid high tag; inert (doc subnet, no clients)
		IsNat:        ptr(true),
		DhcpdEnabled: ptr(false),
	}
	created, err := nets.Create(ctx, base)
	if err != nil {
		t.Fatalf("create baseline network: %v", err)
	}
	id := acctest.Deref(created.UnderscoreId)
	t.Cleanup(func() {
		if err := nets.Delete(context.Background(), id); err != nil && !errors.Is(err, network.ErrNotFound) {
			t.Logf("cleanup delete %s: %v (sweeper backstops)", id, err)
		}
	})

	// A — plain non-subnet update via a sparse PUT must SUCCEED (A is not real).
	if _, err := nets.LegacyCollection.Update(ctx, id,
		&wire.Networkconf{UnderscoreId: ptr(id), Name: ptr(tfaccHILPrefix + "dhcpguard-renamed")}); err != nil {
		t.Errorf("A: sparse non-subnet update should succeed (controller shallow-merges), got: %v", err)
	}

	// B — enabling guard with NO trusted server must be REJECTED with
	// MissingIPAddress (the downstream user's exact failing change).
	if _, err := nets.LegacyCollection.Update(ctx, id,
		&wire.Networkconf{UnderscoreId: ptr(id), DhcpguardEnabled: ptr(true)}); !isMissingIPAddress(err) {
		t.Errorf("B: guard-on with no trusted server should fail MissingIPAddress, got: %v", err)
	}

	// B — the missing field is NOT ip_subnet: re-send the full live doc (which
	// carries ip_subnet) with only dhcpguard overlaid. It STILL fails. This is the
	// load-bearing evidence that PR #92's GET-merge-PUT would not fix B.
	full, err := nets.Get(ctx, id)
	if err != nil {
		t.Fatalf("read full doc: %v", err)
	}
	full.DhcpguardEnabled = ptr(true)
	if acctest.Deref(full.IpSubnet) == "" {
		t.Fatalf("precondition: live doc must carry ip_subnet, got empty")
	}
	if _, err := nets.LegacyCollection.Update(ctx, id, full); !isMissingIPAddress(err) {
		t.Errorf("B: guard-on via full-doc PUT (ip_subnet present) should still fail MissingIPAddress, got: %v", err)
	}

	// B — supplying a trusted server in dhcpd_ip_1 makes guard-on SUCCEED. This is
	// exactly what dhcp_guard_servers → dhcpd_ip_1..3 carries.
	if _, err := nets.LegacyCollection.Update(ctx, id,
		&wire.Networkconf{UnderscoreId: ptr(id), DhcpguardEnabled: ptr(true), DhcpdIp1: ptr("192.0.2.1")}); err != nil {
		t.Errorf("B: guard-on with a trusted server (dhcpd_ip_1) should succeed, got: %v", err)
	}

	// Confirm the controller persisted both the guard flag and the trusted server.
	got, err := nets.Get(ctx, id)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.DhcpguardEnabled == nil || !*got.DhcpguardEnabled {
		t.Errorf("guard flag not persisted: %v", got.DhcpguardEnabled)
	}
	if acctest.Deref(got.DhcpdIp1) != "192.0.2.1" {
		t.Errorf("trusted server not persisted: dhcpd_ip_1=%q", acctest.Deref(got.DhcpdIp1))
	}
}
