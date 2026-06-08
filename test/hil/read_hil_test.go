//go:build hil

package hil

// Tier-3 HIL read-only smoke test (docs/testing.md). Logs in to a LIVE UniFi OS
// console with a local admin and Lists every Network collection, forcing each
// element through its generated `wire` type — so a field our clean-room spec got
// wrong shows up as a decode failure against real firmware.
//
// READ-ONLY BY CONSTRUCTION: it issues only GETs (plus the one session-login
// POST). There is no Create/Update/Delete codepath here. Never writes.
//
// Excluded from normal builds by the `hil` tag, so `make test` never compiles or
// runs it. Run from the workstation that can reach the console; set UNIFI_HOST,
// UNIFI_USERNAME (a local admin), and UNIFI_PASSWORD — e.g. via test/hil/secrets.env
// (see test/hil/secrets.env.example), which test/hil/run.sh sources for you:
//
//	UNIFI_ALLOW_INSECURE=true \
//	  go test -tags hil ./test/hil -run TestHILRead -count=1 -v

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dovys/terraform-provider-unifi/internal/acctest"
	"github.com/dovys/terraform-provider-unifi/internal/unifi"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

func TestHILRead(t *testing.T) {
	host := os.Getenv("UNIFI_HOST")
	if host == "" {
		t.Skip("set UNIFI_HOST to run the HIL read smoke test")
	}
	user, pass := os.Getenv("UNIFI_USERNAME"), os.Getenv("UNIFI_PASSWORD")
	if user == "" || pass == "" {
		t.Fatal("set UNIFI_USERNAME and UNIFI_PASSWORD (a local UniFi OS admin)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	site := acctest.EnvOr("UNIFI_SITE", "default")
	c, err := unifi.NewClient(ctx, unifi.Config{
		Host:          host,
		Username:      user,
		Password:      pass,
		Site:          site,
		AllowInsecure: strings.EqualFold(os.Getenv("UNIFI_ALLOW_INSECURE"), "true") || os.Getenv("UNIFI_ALLOW_INSECURE") == "1",
	})
	if err != nil {
		t.Fatalf("login/connect to %s failed: %v", host, err)
	}
	t.Logf("connected to %s (site %q)", host, site)

	n := c.Network()

	type result struct {
		name   string
		count  int
		status string // OK | EMPTY | HTTP nnn | DECODE-FAIL | FAIL
		err    error
	}
	var results []result
	probe := func(name string, list func() (int, error)) {
		count, err := list()
		r := result{name: name, count: count, err: err}
		switch {
		case err == nil && count == 0:
			r.status = "EMPTY"
		case err == nil:
			r.status = "OK"
		default:
			var apiErr *transport.APIError
			var typeErr *json.UnmarshalTypeError
			var synErr *json.SyntaxError
			switch {
			case errors.As(err, &typeErr), errors.As(err, &synErr):
				r.status = "DECODE-FAIL" // wire type mismatch vs live firmware — a real bug
			case errors.As(err, &apiErr):
				r.status = fmt.Sprintf("HTTP %d", apiErr.StatusCode) // endpoint behavior, not a wire bug
			default:
				r.status = "FAIL"
			}
		}
		results = append(results, r)
	}

	// Every read-only collection on the Network client. len() forces a full
	// decode of each element through its generated wire type.
	probe("networks", func() (int, error) { v, e := n.Networks().List(ctx); return len(v), e })
	probe("wan.network_groups", func() (int, error) { v, e := n.WAN().ListNetworkGroups(ctx); return len(v), e })
	probe("wan.slas", func() (int, error) { v, e := n.WAN().SLAs().List(ctx); return len(v), e })
	probe("wifi.wlans", func() (int, error) { v, e := n.WiFi().WLANs().List(ctx); return len(v), e })
	probe("wifi.wlan_groups", func() (int, error) { v, e := n.WiFi().WLANGroups().List(ctx); return len(v), e })
	probe("wifi.ap_groups", func() (int, error) { v, e := n.WiFi().APGroups().List(ctx); return len(v), e })
	probe("firewall.groups", func() (int, error) { v, e := n.Firewall().Groups().List(ctx); return len(v), e })
	probe("firewall.rules", func() (int, error) { v, e := n.Firewall().Rules().List(ctx); return len(v), e })
	probe("devices", func() (int, error) { v, e := n.Devices().List(ctx); return len(v), e })
	probe("clients.users", func() (int, error) { v, e := n.Clients().Users().List(ctx); return len(v), e })
	probe("user_groups", func() (int, error) { v, e := n.UserGroups().List(ctx); return len(v), e })
	probe("radius.profiles", func() (int, error) { v, e := n.RADIUS().Profiles().List(ctx); return len(v), e })
	probe("radius.accounts", func() (int, error) { v, e := n.RADIUS().Accounts().List(ctx); return len(v), e })
	probe("dns.records", func() (int, error) { v, e := n.DNS().Records().List(ctx); return len(v), e })
	probe("port_profiles", func() (int, error) { v, e := n.PortProfiles().List(ctx); return len(v), e })
	probe("port_forwards", func() (int, error) { v, e := n.PortForwards().List(ctx); return len(v), e })
	probe("static_routes", func() (int, error) { v, e := n.StaticRoutes().List(ctx); return len(v), e })
	probe("dynamic_dns", func() (int, error) { v, e := n.DynamicDNS().List(ctx); return len(v), e })
	probe("dpi.apps", func() (int, error) { v, e := n.DPIApps().List(ctx); return len(v), e })
	probe("dpi.groups", func() (int, error) { v, e := n.DPIGroups().List(ctx); return len(v), e })
	probe("traffic.rules", func() (int, error) { v, e := n.Traffic().Rules().List(ctx); return len(v), e })
	probe("traffic.routes", func() (int, error) { v, e := n.Traffic().Routes().List(ctx); return len(v), e })
	probe("traffic.qos", func() (int, error) { v, e := n.Traffic().QoS().List(ctx); return len(v), e })

	// Protect app (read-only) — same authenticated session, no api_key. Skip
	// cleanly when Protect isn't installed (the NVR version probe the console
	// session itself uses on connect). When it IS installed, each List/Get forces
	// a full decode through the Protect wire types — so a spec/wire field our
	// clean-room model got wrong surfaces as DECODE-FAIL against real Protect
	// firmware, exactly as for the Network collections above. lights/sensors/
	// chimes/viewers are typically EMPTY (no such accessory on the recon console),
	// which still confirms the endpoint + empty-decode.
	if _, perr := c.Protect().NVR().Version(ctx); perr != nil {
		t.Logf("Protect not installed/reachable — skipping Protect probes: %v", perr)
	} else {
		p := c.Protect()
		probe("protect.nvr", func() (int, error) {
			if _, e := p.NVR().Get(ctx); e != nil {
				return 0, e
			}
			return 1, nil
		})
		probe("protect.cameras", func() (int, error) { v, e := p.Cameras().List(ctx); return len(v), e })
		probe("protect.lights", func() (int, error) { v, e := p.Lights().List(ctx); return len(v), e })
		probe("protect.sensors", func() (int, error) { v, e := p.Sensors().List(ctx); return len(v), e })
		probe("protect.chimes", func() (int, error) { v, e := p.Chimes().List(ctx); return len(v), e })
		probe("protect.viewers", func() (int, error) { v, e := p.Viewers().List(ctx); return len(v), e })
		probe("protect.liveviews", func() (int, error) { v, e := p.Liveviews().List(ctx); return len(v), e })
	}

	// Console OS surface (read-only) — unifi-core /api/userCertificates over the
	// same session. len() forces a full decode through console.Certificate, so a
	// hand-written wire field that disagrees with real firmware surfaces here.
	// Typically EMPTY while the console is on its self-signed default.
	probe("console.user_certificates", func() (int, error) { v, e := c.Console().Certificates().List(ctx); return len(v), e })

	sort.Slice(results, func(i, j int) bool { return results[i].name < results[j].name })

	var decodeFails, otherFails int
	t.Log("─── HIL read smoke test ──────────────────────────────")
	for _, r := range results {
		switch r.status {
		case "OK":
			t.Logf("  ✓ %-20s %d", r.name, r.count)
		case "EMPTY":
			t.Logf("  · %-20s 0 (empty)", r.name)
		case "DECODE-FAIL":
			decodeFails++
			t.Logf("  ✗ %-20s DECODE-FAIL: %v", r.name, r.err)
		case "FAIL":
			otherFails++
			t.Logf("  ✗ %-20s FAIL: %v", r.name, r.err)
		default: // HTTP nnn — endpoint absent/unsupported on this firmware
			t.Logf("  ! %-20s %s (endpoint absent/unsupported): %v", r.name, r.status, r.err)
		}
	}
	t.Logf("─── %d collections probed ────────────────────────────", len(results))

	if decodeFails > 0 {
		t.Errorf("%d collection(s) failed to deserialize — wire type mismatch vs live firmware", decodeFails)
	}
	if otherFails > 0 {
		t.Errorf("%d collection(s) errored unexpectedly (non-HTTP, non-decode)", otherFails)
	}
}
