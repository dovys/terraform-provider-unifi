package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

func TestProviderMetadata(t *testing.T) {
	p := New("1.2.3")()
	resp := &provider.MetadataResponse{}
	p.Metadata(context.Background(), provider.MetadataRequest{}, resp)
	if resp.TypeName != "unifi" {
		t.Errorf("TypeName = %q, want unifi", resp.TypeName)
	}
	if resp.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", resp.Version)
	}
}

func TestProviderSchema(t *testing.T) {
	p := New("test")()
	resp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	attrs := resp.Schema.Attributes
	for _, name := range []string{"host", "api_key", "username", "password", "site", "allow_insecure", "timeout", "max_retries"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
	for _, name := range []string{"api_key", "password"} {
		if a, ok := attrs[name]; ok && !a.IsSensitive() {
			t.Errorf("attribute %q is not marked Sensitive", name)
		}
	}
}

func TestDescribeClientError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		summary string
	}{
		{"2fa", &transport.TwoFactorError{}, "2FA"},
		{"auth", &transport.AuthError{StatusCode: 401}, "login failed"},
		{"config", &unifi.ConfigError{Msg: "no host"}, "configuration error"},
		{"capability", &unifi.CapabilityError{Capability: "x"}, "capability unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, detail := describeClientError(tt.err)
			if !strings.Contains(summary, tt.summary) {
				t.Errorf("summary = %q, want to contain %q", summary, tt.summary)
			}
			if detail == "" {
				t.Error("detail is empty")
			}
		})
	}
}
