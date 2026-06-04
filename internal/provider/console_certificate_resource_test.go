package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/console"
)

func TestConsoleCertificateSchema_WriteOnly(t *testing.T) {
	var resp resource.SchemaResponse
	NewConsoleCertificateResource().(*consoleCertificateResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "name", "source", "domain", "dns_provider", "certificate", "private_key", "valid_to", "active", "acme_renew_error"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
	// private_key and the whole dns_provider block (incl. leaves) must be write-only —
	// the property that keeps secrets out of state and re-applies a no-op.
	if !resp.Schema.Attributes["private_key"].IsWriteOnly() {
		t.Error("private_key must be write-only")
	}
	dns, ok := resp.Schema.Attributes["dns_provider"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("dns_provider is %T, want SingleNestedAttribute", resp.Schema.Attributes["dns_provider"])
	}
	if !dns.IsWriteOnly() {
		t.Error("dns_provider block must be write-only")
	}
	for name, a := range dns.Attributes {
		if !a.IsWriteOnly() {
			t.Errorf("dns_provider.%s must be write-only", name)
		}
	}
	// certificate is intentionally NOT write-only (stored+diffed for chaining).
	if resp.Schema.Attributes["certificate"].IsWriteOnly() {
		t.Error("certificate must NOT be write-only (it is stored and diffed)")
	}
}

func diagsContain(d diag.Diagnostics, summarySubstr string) bool {
	for _, e := range d.Errors() {
		if strings.Contains(e.Summary(), summarySubstr) {
			return true
		}
	}
	return false
}

func TestValidateConsoleCertConfig_Matrix(t *testing.T) {
	cf := func() *dnsProviderModel {
		return &dnsProviderModel{Name: types.StringValue(console.ProviderCloudflare), APIToken: types.StringValue("tok")}
	}
	null := types.StringNull()

	tests := []struct {
		name      string
		cfg       consoleCertificateResourceModel
		wantError bool
		wantMsg   string
	}{
		{
			name: "letsEncrypt cloudflare ok",
			cfg:  consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com"), DNSProvider: cf()},
		},
		{
			name: "source null defaults to letsEncrypt, needs domain",
			cfg:  consoleCertificateResourceModel{Source: null, DNSProvider: cf()},
			// domain missing
			wantError: true, wantMsg: "Missing domain",
		},
		{
			name:      "letsEncrypt missing dns_provider",
			cfg:       consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com")},
			wantError: true, wantMsg: "Missing dns_provider",
		},
		{
			name: "letsEncrypt with stray certificate",
			cfg: consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com"),
				DNSProvider: cf(), Certificate: types.StringValue("PEM")},
			wantError: true, wantMsg: "Unexpected certificate",
		},
		{
			name: "upload ok",
			cfg:  consoleCertificateResourceModel{Source: types.StringValue(sourceUpload), Certificate: types.StringValue("PEM"), PrivateKey: types.StringValue("KEY")},
		},
		{
			name:      "upload missing private_key",
			cfg:       consoleCertificateResourceModel{Source: types.StringValue(sourceUpload), Certificate: types.StringValue("PEM")},
			wantError: true, wantMsg: "Missing private key",
		},
		{
			name: "upload with stray dns_provider",
			cfg: consoleCertificateResourceModel{Source: types.StringValue(sourceUpload), Certificate: types.StringValue("PEM"),
				PrivateKey: types.StringValue("KEY"), DNSProvider: cf()},
			wantError: true, wantMsg: "Unexpected dns_provider",
		},
		{
			// Write-only credentials read as *unknown* at plan time — that must count
			// as present, not missing (otherwise every plan would fail validation).
			name: "cloudflare unknown api_token (write-only at plan) ok",
			cfg: consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com"),
				DNSProvider: &dnsProviderModel{Name: types.StringValue(console.ProviderCloudflare), APIToken: types.StringUnknown()}},
		},
		{
			name: "cloudflare missing api_token",
			cfg: consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com"),
				DNSProvider: &dnsProviderModel{Name: types.StringValue(console.ProviderCloudflare)}},
			wantError: true, wantMsg: "Missing credential",
		},
		{
			name: "cloudflare with stray secret_key",
			cfg: consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com"),
				DNSProvider: &dnsProviderModel{Name: types.StringValue(console.ProviderCloudflare), APIToken: types.StringValue("t"), SecretKey: types.StringValue("x")}},
			wantError: true, wantMsg: "Unexpected credential",
		},
		{
			name: "route53 needs three fields",
			cfg: consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com"),
				DNSProvider: &dnsProviderModel{Name: types.StringValue(console.ProviderRoute53), AccessKeyID: types.StringValue("a"), SecretAccessKey: types.StringValue("s")}},
			wantError: true, wantMsg: "Missing credential", // region missing
		},
		{
			name: "route53 complete ok",
			cfg: consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com"),
				DNSProvider: &dnsProviderModel{Name: types.StringValue(console.ProviderRoute53), AccessKeyID: types.StringValue("a"), SecretAccessKey: types.StringValue("s"), Region: types.StringValue("us-east-1")}},
		},
		{
			name: "manual needs no creds",
			cfg: consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com"),
				DNSProvider: &dnsProviderModel{Name: types.StringValue(console.ProviderManual)}},
		},
		{
			name: "manual rejects creds",
			cfg: consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c.example.com"),
				DNSProvider: &dnsProviderModel{Name: types.StringValue(console.ProviderManual), APIToken: types.StringValue("t")}},
			wantError: true, wantMsg: "Unexpected credential",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := validateConsoleCertConfig(tc.cfg)
			if tc.wantError != d.HasError() {
				t.Fatalf("HasError = %v, want %v (diags: %v)", d.HasError(), tc.wantError, d)
			}
			if tc.wantMsg != "" && !diagsContain(d, tc.wantMsg) {
				t.Fatalf("diags %v do not contain %q", d, tc.wantMsg)
			}
		})
	}
}

func TestDNSProviderToWire(t *testing.T) {
	// cloudflare: only provider + apiToken set.
	w := (&dnsProviderModel{Name: types.StringValue(console.ProviderCloudflare), APIToken: types.StringValue("tok")}).toWire()
	if w == nil || w.Provider != "cloudflare" || w.APIToken != "tok" {
		t.Fatalf("cloudflare wire = %+v", w)
	}
	if w.APIKey != "" || w.SecretKey != "" || w.Region != "" {
		t.Errorf("cloudflare wire leaked other fields: %+v", w)
	}

	// route53: three fields.
	w = (&dnsProviderModel{Name: types.StringValue(console.ProviderRoute53), AccessKeyID: types.StringValue("a"), SecretAccessKey: types.StringValue("s"), Region: types.StringValue("eu-west-1")}).toWire()
	if w.AccessKeyID != "a" || w.SecretAccessKey != "s" || w.Region != "eu-west-1" {
		t.Errorf("route53 wire = %+v", w)
	}

	// ovh: four fields.
	w = (&dnsProviderModel{Name: types.StringValue(console.ProviderOVH), ApplicationKey: types.StringValue("ak"), ApplicationSecret: types.StringValue("as"), ConsumerKey: types.StringValue("ck"), Endpoint: types.StringValue("ovh-eu")}).toWire()
	if w.ApplicationKey != "ak" || w.ApplicationSecret != "as" || w.ConsumerKey != "ck" || w.Endpoint != "ovh-eu" {
		t.Errorf("ovh wire = %+v", w)
	}

	// manual → nil (no provider object is sent).
	if got := (&dnsProviderModel{Name: types.StringValue(console.ProviderManual)}).toWire(); got != nil {
		t.Errorf("manual toWire = %+v, want nil", got)
	}
	// nil receiver → nil.
	if got := (*dnsProviderModel)(nil).toWire(); got != nil {
		t.Errorf("nil toWire = %+v, want nil", got)
	}
}

func TestFromCertificate(t *testing.T) {
	cert := &console.Certificate{
		ID:           "cert-1",
		Name:         "console",
		Active:       true,
		Version:      3,
		SerialNumber: "0A:0B",
		Fingerprint:  "ff:ee",
		Subject:      console.CertificateAttributes{CN: "console.example.com"},
		Issuer:       console.CertificateAttributes{CN: "E5"},
		ValidFrom:    "2026-06-01T00:00:00Z",
		ValidTo:      "2026-09-01T00:00:00Z",
		Source:       "lets_encrypt",
	}

	// Managed LE resource (domain already set) — computed fields refresh, domain unchanged.
	m := consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("console.example.com")}
	m.fromCertificate(cert)
	if m.ID.ValueString() != "cert-1" || !m.Active.ValueBool() || m.ValidTo.ValueString() != "2026-09-01T00:00:00Z" {
		t.Fatalf("computed fields wrong: %+v", m)
	}
	if m.Source.ValueString() != sourceLetsEncrypt {
		t.Errorf("source = %q, want letsEncrypt", m.Source.ValueString())
	}
	if m.CertVersion.ValueInt64() != 3 {
		t.Errorf("cert_version = %d, want 3", m.CertVersion.ValueInt64())
	}
	if m.ACMERenewError.ValueString() != "" {
		t.Errorf("acme_renew_error = %q, want empty", m.ACMERenewError.ValueString())
	}

	// Import case (empty domain): recover it from the subject CN.
	imp := consoleCertificateResourceModel{}
	imp.fromCertificate(cert)
	if imp.Domain.ValueString() != "console.example.com" {
		t.Errorf("imported domain = %q, want console.example.com (from CN)", imp.Domain.ValueString())
	}

	// Uploaded cert: source maps to upload, domain stays null (no CN recovery).
	up := consoleCertificateResourceModel{}
	up.fromCertificate(&console.Certificate{ID: "u", Name: "byo", Source: "uploaded", Subject: console.CertificateAttributes{CN: "byo.example.com"}})
	if up.Source.ValueString() != sourceUpload {
		t.Errorf("uploaded source = %q, want upload", up.Source.ValueString())
	}
	if !up.Domain.IsNull() {
		t.Errorf("uploaded domain = %q, want null", up.Domain.ValueString())
	}

	// Renewal error surfaces its code.
	re := consoleCertificateResourceModel{Source: types.StringValue(sourceLetsEncrypt), Domain: types.StringValue("c")}
	re.fromCertificate(&console.Certificate{ID: "x", Source: "lets_encrypt", ACMERenewError: &console.ACMEError{Code: console.ErrCodeACMECertificateRenewTooEarly, Message: "too early"}})
	if re.ACMERenewError.ValueString() != console.ErrCodeACMECertificateRenewTooEarly {
		t.Errorf("acme_renew_error = %q", re.ACMERenewError.ValueString())
	}
}
