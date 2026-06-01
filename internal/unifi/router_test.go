package unifi

import "testing"

func TestRouterBackend(t *testing.T) {
	preFloor := NewRouter(FeatureFlags{})                                                       // older firmware
	postFloor := NewRouter(FeatureFlags{IntegrationWrites: true, IntegrationDNSPolicies: true}) // 10.1.84+

	tests := []struct {
		name   string
		router *Router
		op     Op
		want   Backend
	}{
		{"networks always legacy (pre)", preFloor, OpNetworkWrite, BackendLegacy},
		{"networks always legacy (post)", postFloor, OpNetworkWrite, BackendLegacy},
		{"ap_group v2", preFloor, OpAPGroup, BackendV2},
		{"zone firewall v2", preFloor, OpFirewallZone, BackendV2},
		{"traffic v2", postFloor, OpTraffic, BackendV2},
		{"site read integration", preFloor, OpSiteRead, BackendIntegration},
		{"voucher integration", preFloor, OpVoucher, BackendIntegration},
		{"device read legacy", postFloor, OpDeviceRead, BackendLegacy},
		{"device config legacy", postFloor, OpDeviceConfig, BackendLegacy},
		{"local DNS v2 pre-floor", preFloor, OpLocalDNS, BackendV2},
		{"local DNS integration post-floor", postFloor, OpLocalDNS, BackendIntegration},
		{"adopt legacy pre-floor", preFloor, OpDeviceAdopt, BackendLegacy},
		{"adopt integration post-floor", postFloor, OpDeviceAdopt, BackendIntegration},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.router.Backend(tt.op); got != tt.want {
				t.Errorf("Backend(%s) = %s, want %s", tt.op, got, tt.want)
			}
		})
	}
}

func TestRouterIntegrationAvailable(t *testing.T) {
	if NewRouter(FeatureFlags{}).IntegrationAvailable() {
		t.Error("IntegrationAvailable() = true on conservative flags, want false")
	}
	if !NewRouter(FeatureFlags{IntegrationWrites: true}).IntegrationAvailable() {
		t.Error("IntegrationAvailable() = false when IntegrationWrites set, want true")
	}
}

func TestBackendString(t *testing.T) {
	for b, want := range map[Backend]string{
		BackendLegacy:      "legacy",
		BackendV2:          "v2",
		BackendIntegration: "integration",
	} {
		if got := b.String(); got != want {
			t.Errorf("Backend(%d).String() = %q, want %q", b, got, want)
		}
	}
}
