package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/console"
)

var (
	_ resource.Resource                   = (*consoleCertificateResource)(nil)
	_ resource.ResourceWithConfigure      = (*consoleCertificateResource)(nil)
	_ resource.ResourceWithImportState    = (*consoleCertificateResource)(nil)
	_ resource.ResourceWithValidateConfig = (*consoleCertificateResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*consoleCertificateResource)(nil)
)

// oneOfList renders a backtick-quoted, comma-separated list for documentation.
func oneOfList(vals []string) string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = "`" + v + "`"
	}
	return strings.Join(out, ", ")
}

const (
	sourceLetsEncrypt = "letsEncrypt"
	sourceUpload      = "upload"
)

// awsRegions is the Route53 region enum the console UI ships (DNS-01 hosted-zone
// region); ovhEndpoints is the OVH API endpoint enum; dnsProviderNames is the
// DNS-01 provider discriminator set.
var (
	awsRegions = []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"af-south-1", "ap-east-1", "ap-east-2",
		"ap-south-1", "ap-south-2",
		"ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-4",
		"ap-southeast-5", "ap-southeast-6", "ap-southeast-7",
		"ap-northeast-1", "ap-northeast-2", "ap-northeast-3",
		"ca-central-1", "ca-west-1",
		"eu-central-1", "eu-central-2", "eu-west-1", "eu-west-2", "eu-west-3",
		"eu-south-1", "eu-south-2", "eu-north-1",
		"il-central-1", "mx-central-1", "me-south-1", "me-central-1", "sa-east-1",
	}
	ovhEndpoints = []string{
		"ovh-eu", "ovh-us", "ovh-ca",
		"kimsufi-eu", "kimsufi-ca",
		"soyoustart-eu", "soyoustart-ca",
	}
	dnsProviderNames = []string{
		console.ProviderCloudflare, console.ProviderRoute53, console.ProviderGoDaddy,
		console.ProviderDigitalOcean, console.ProviderPorkbun, console.ProviderDuckDNS,
		console.ProviderDynu, console.ProviderOVH, console.ProviderManual,
	}
)

func NewConsoleCertificateResource() resource.Resource { return &consoleCertificateResource{} }

type consoleCertificateResource struct {
	client unifi.Client
}

type consoleCertificateResourceModel struct {
	ID                types.String      `tfsdk:"id"`
	Name              types.String      `tfsdk:"name"`
	Source            types.String      `tfsdk:"source"`
	Domain            types.String      `tfsdk:"domain"`
	DNSProvider       *dnsProviderModel `tfsdk:"dns_provider"`
	Certificate       types.String      `tfsdk:"certificate"`
	PrivateKey        types.String      `tfsdk:"private_key"`
	Active            types.Bool        `tfsdk:"active"`
	SerialNumber      types.String      `tfsdk:"serial_number"`
	Fingerprint       types.String      `tfsdk:"fingerprint"`
	SubjectCommonName types.String      `tfsdk:"subject_common_name"`
	Issuer            types.String      `tfsdk:"issuer"`
	ValidFrom         types.String      `tfsdk:"valid_from"`
	ValidTo           types.String      `tfsdk:"valid_to"`
	CertVersion       types.Int64       `tfsdk:"cert_version"`
	ACMERenewError    types.String      `tfsdk:"acme_renew_error"`
}

// dnsProviderModel mirrors the console's discriminated DNS-provider union. The
// whole block is write-only: it is intent handed to the console once (the device
// owns renewal after that), and it can't be read back, so it is never stored in
// state nor diffed — which keeps a plain re-apply a no-op and import clean.
type dnsProviderModel struct {
	Name              types.String `tfsdk:"name"`
	APIToken          types.String `tfsdk:"api_token"`
	Token             types.String `tfsdk:"token"`
	APIKey            types.String `tfsdk:"api_key"`
	APISecret         types.String `tfsdk:"api_secret"`
	SecretKey         types.String `tfsdk:"secret_key"`
	AccessKeyID       types.String `tfsdk:"access_key_id"`
	SecretAccessKey   types.String `tfsdk:"secret_access_key"`
	Region            types.String `tfsdk:"region"`
	ApplicationKey    types.String `tfsdk:"application_key"`
	ApplicationSecret types.String `tfsdk:"application_secret"`
	ConsumerKey       types.String `tfsdk:"consumer_key"`
	Endpoint          types.String `tfsdk:"endpoint"`
}

func (r *consoleCertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_console_certificate"
}

func (r *consoleCertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	writeOnlySecret := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A TLS certificate installed on the **UniFi OS console itself** (the unifi-core management UI, " +
			"e.g. `https://192.0.2.1`) — replacing the self-signed default with a Let's Encrypt certificate (native " +
			"ACME, automated DNS-01) or an uploaded BYO certificate.\n\n" +
			"For `letsEncrypt`, the **console owns renewal**: Terraform performs the initial issuance and hands the " +
			"credentials to the device, which then renews on its own. Terraform never re-issues on a plain `apply` — " +
			"DNS credentials are **write-only** (never stored in state, never diffed). To rotate a token or switch " +
			"provider, recreate the resource (`terraform apply -replace=...`).\n\n" +
			"Requires Terraform >= 1.11 / OpenTofu >= 1.9 (write-only arguments).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Console certificate ID. Import key and state `id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Certificate display name on the console. Immutable (no rename API) — changing it replaces the certificate.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"source": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(sourceLetsEncrypt),
				MarkdownDescription: "How the certificate is obtained: `letsEncrypt` (native ACME via DNS-01; default) or " +
					"`upload` (bring your own). Immutable — changing it replaces the certificate.",
				Validators:    []validator.String{stringvalidator.OneOf(sourceLetsEncrypt, sourceUpload)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"domain": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Fully-qualified domain for the certificate (e.g. `console.example.com`; a wildcard " +
					"`*.example.com` is allowed). Required for `source = letsEncrypt`. Immutable — changing it replaces the certificate.",
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 253)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"dns_provider": schema.SingleNestedAttribute{
				Optional:  true,
				WriteOnly: true,
				MarkdownDescription: "DNS provider for automated DNS-01 (`source = letsEncrypt`). The console calls the " +
					"provider's API itself to place the `_acme-challenge` TXT record. **Write-only**: credentials are " +
					"consumed at issuance and never stored in state or read back. Use `name = manual` to place the TXT " +
					"yourself (the device cannot do it for you).",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Required:            true,
						WriteOnly:           true,
						MarkdownDescription: "DNS provider: one of " + oneOfList(dnsProviderNames) + ".",
						Validators:          []validator.String{stringvalidator.OneOf(dnsProviderNames...)},
					},
					"api_token":          writeOnlySecret("API token — `cloudflare`, `digitalocean`."),
					"token":              writeOnlySecret("Token — `duckdns`."),
					"api_key":            writeOnlySecret("API key — `dynu`, `godaddy`, `porkbun`."),
					"api_secret":         writeOnlySecret("API secret — `godaddy`."),
					"secret_key":         writeOnlySecret("Secret key — `porkbun`."),
					"access_key_id":      writeOnlySecret("Access key ID — `route53`."),
					"secret_access_key":  writeOnlySecret("Secret access key — `route53`."),
					"application_key":    writeOnlySecret("Application key — `ovh`."),
					"application_secret": writeOnlySecret("Application secret — `ovh`."),
					"consumer_key":       writeOnlySecret("Consumer key — `ovh`."),
					"region": schema.StringAttribute{
						Optional:            true,
						WriteOnly:           true,
						MarkdownDescription: "Hosted-zone region — `route53`.",
						Validators:          []validator.String{stringvalidator.OneOf(awsRegions...)},
					},
					"endpoint": schema.StringAttribute{
						Optional:            true,
						WriteOnly:           true,
						MarkdownDescription: "API endpoint — `ovh`: one of " + oneOfList(ovhEndpoints) + ".",
						Validators:          []validator.String{stringvalidator.OneOf(ovhEndpoints...)},
					},
				},
			},
			"certificate": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "PEM-encoded certificate (full chain) — required for `source = upload`. Stored and " +
					"diffed so it chains with an upstream `acme_certificate`/`tls_*` resource: when the PEM changes the " +
					"certificate is re-uploaded. (The console never returns the PEM, so importing a pre-existing uploaded " +
					"certificate shows a one-time re-upload diff.)",
			},
			"private_key": writeOnlySecret("PEM-encoded private key — required for `source = upload`. Write-only: never stored in state."),
			"active": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
				MarkdownDescription: "Whether this certificate is the **active (serving)** console certificate. Defaults to " +
					"`true` — the resource activates the certificate it installs so the console actually serves it (a " +
					"freshly issued/uploaded certificate is inactive until activated). Set `false` to install/stage a " +
					"certificate without serving it.",
			},
			"serial_number":       computedString("Certificate serial number."),
			"fingerprint":         computedString("Certificate fingerprint."),
			"subject_common_name": computedString("Subject common name (CN)."),
			"issuer":              computedString("Issuer common name (CN)."),
			"valid_from":          computedString("Not-before timestamp (RFC 3339)."),
			"valid_to": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Not-after (expiry) timestamp (RFC 3339). Refreshed on every read; because the device " +
					"renews natively under the same `id`, expiry drift never produces a plan diff.",
			},
			"cert_version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Certificate version counter reported by the console (bumps on each native renewal).",
			},
			"acme_renew_error": computedString("Reason the device's last native renewal failed, if any (empty when healthy)."),
		},
	}
}

func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{Computed: true, MarkdownDescription: desc}
}

func (r *consoleCertificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *consoleCertificateResource) ModifyPlan(_ context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || r.client == nil {
		return // destroy, or not yet configured (early validation walk)
	}
	requireConsoleCert(r.client, &resp.Diagnostics)
}

// ValidateConfig enforces the per-source, per-provider field matrix the
// controller expects (so a misconfiguration is a plan-time message, not a
// runtime 4xx).
func (r *consoleCertificateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg consoleCertificateResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateConsoleCertConfig(cfg)...)
}

// validateConsoleCertConfig is the pure config-validation logic (extracted so it
// is unit-testable without constructing a tfsdk.Config).
func validateConsoleCertConfig(cfg consoleCertificateResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	// source is Optional+Computed: a null config value means the default
	// (letsEncrypt). Skip while unknown (interpolated).
	source := sourceLetsEncrypt
	if !cfg.Source.IsUnknown() && !cfg.Source.IsNull() {
		source = cfg.Source.ValueString()
	}

	switch source {
	case sourceUpload:
		if cfg.Certificate.IsNull() {
			diags.AddAttributeError(path.Root("certificate"), "Missing certificate",
				"`certificate` is required when `source = upload`.")
		}
		if cfg.PrivateKey.IsNull() {
			diags.AddAttributeError(path.Root("private_key"), "Missing private key",
				"`private_key` is required when `source = upload`.")
		}
		if !cfg.Domain.IsNull() {
			diags.AddAttributeError(path.Root("domain"), "Unexpected domain",
				"`domain` applies only to `source = letsEncrypt`; remove it for an uploaded certificate.")
		}
		if cfg.DNSProvider != nil {
			diags.AddAttributeError(path.Root("dns_provider"), "Unexpected dns_provider",
				"`dns_provider` applies only to `source = letsEncrypt`; remove it for an uploaded certificate.")
		}
	default: // letsEncrypt
		if cfg.Domain.IsNull() {
			diags.AddAttributeError(path.Root("domain"), "Missing domain",
				"`domain` is required when `source = letsEncrypt`.")
		}
		if cfg.DNSProvider == nil {
			diags.AddAttributeError(path.Root("dns_provider"), "Missing dns_provider",
				"`dns_provider` is required when `source = letsEncrypt` (use `name = manual` to place the TXT record yourself).")
		} else {
			validateDNSProvider(cfg.DNSProvider, &diags)
		}
		if !cfg.Certificate.IsNull() {
			diags.AddAttributeError(path.Root("certificate"), "Unexpected certificate",
				"`certificate` applies only to `source = upload`; for Let's Encrypt the console issues the certificate.")
		}
		if !cfg.PrivateKey.IsNull() {
			diags.AddAttributeError(path.Root("private_key"), "Unexpected private_key",
				"`private_key` applies only to `source = upload`.")
		}
	}
	return diags
}

// validateDNSProvider checks that exactly the selected provider's credential
// fields are present (required ones set, others empty).
func validateDNSProvider(m *dnsProviderModel, diags *diag.Diagnostics) {
	if m.Name.IsUnknown() || m.Name.IsNull() {
		return
	}
	// required[field] for the selected provider; every other credential must be unset.
	type field struct {
		name string
		val  types.String
	}
	all := []field{
		{"api_token", m.APIToken}, {"token", m.Token}, {"api_key", m.APIKey},
		{"api_secret", m.APISecret}, {"secret_key", m.SecretKey},
		{"access_key_id", m.AccessKeyID}, {"secret_access_key", m.SecretAccessKey}, {"region", m.Region},
		{"application_key", m.ApplicationKey}, {"application_secret", m.ApplicationSecret},
		{"consumer_key", m.ConsumerKey}, {"endpoint", m.Endpoint},
	}
	required := map[string][]string{
		console.ProviderCloudflare:   {"api_token"},
		console.ProviderDigitalOcean: {"api_token"},
		console.ProviderDuckDNS:      {"token"},
		console.ProviderDynu:         {"api_key"},
		console.ProviderGoDaddy:      {"api_key", "api_secret"},
		console.ProviderPorkbun:      {"api_key", "secret_key"},
		console.ProviderRoute53:      {"access_key_id", "secret_access_key", "region"},
		console.ProviderOVH:          {"application_key", "application_secret", "consumer_key", "endpoint"},
		console.ProviderManual:       {},
	}
	name := m.Name.ValueString()
	want, ok := required[name]
	if !ok {
		return // OneOf validator already flags an unknown provider
	}
	wantSet := map[string]bool{}
	for _, f := range want {
		wantSet[f] = true
	}
	for _, f := range all {
		// Write-only credentials read as *unknown* at plan time (their value is
		// only materialized at apply), so "present in config" means not-null —
		// unknown counts as set, only an absent attribute is null.
		set := !f.val.IsNull()
		switch {
		case wantSet[f.name] && !set:
			diags.AddAttributeError(path.Root("dns_provider").AtName(f.name), "Missing credential",
				fmt.Sprintf("`dns_provider.%s` is required for provider %q.", f.name, name))
		case !wantSet[f.name] && set:
			diags.AddAttributeError(path.Root("dns_provider").AtName(f.name), "Unexpected credential",
				fmt.Sprintf("`dns_provider.%s` does not apply to provider %q.", f.name, name))
		}
	}
}

func (r *consoleCertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config consoleCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)     // stored fields (write-only are null here)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...) // write-only fields live here
	if resp.Diagnostics.HasError() {
		return
	}
	certs := r.client.Console().Certificates()

	var issued *console.Certificate
	var err error
	if plan.Source.ValueString() == sourceUpload {
		issued, err = certs.Upload(ctx, console.UploadRequest{
			Name: plan.Name.ValueString(),
			Cert: strOrEmpty(plan.Certificate),
			Key:  strOrEmpty(config.PrivateKey),
		})
		if err != nil {
			addConsoleCertError(&resp.Diagnostics, "upload certificate", err)
			return
		}
	} else {
		issued, err = r.issueLetsEncrypt(ctx, certs, plan, config.DNSProvider)
		if err != nil {
			addConsoleCertError(&resp.Diagnostics, "issue Let's Encrypt certificate", err)
			return
		}
	}

	// A freshly issued/uploaded certificate is inactive — activate it (unless the
	// user staged it with active = false) so the console actually serves it.
	issued, err = ensureActive(ctx, certs, issued, plan.Active.ValueBool())
	if err != nil {
		addConsoleCertError(&resp.Diagnostics, "activate certificate", err)
		return
	}

	plan.fromCertificate(issued)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ensureActive makes the certificate's active (serving) state match desired,
// returning the refreshed certificate.
func ensureActive(ctx context.Context, certs console.CertificateService, cert *console.Certificate, desired bool) (*console.Certificate, error) {
	if cert.Active == desired {
		return cert, nil
	}
	if err := certs.SetActive(ctx, cert.ID, desired); err != nil {
		return nil, err
	}
	return certs.Get(ctx, cert.ID)
}

// issueLetsEncrypt runs the singleton challenge chain: start (the device places
// the TXT for an automated provider) then complete (retrying while DNS
// propagates). A stale in-progress challenge is cancelled and restarted once.
func (r *consoleCertificateResource) issueLetsEncrypt(ctx context.Context, certs console.CertificateService, plan consoleCertificateResourceModel, dns *dnsProviderModel) (*console.Certificate, error) {
	startReq := console.ChallengeStartRequest{
		Name:        plan.Name.ValueString(),
		Domain:      plan.Domain.ValueString(),
		DNSProvider: dns.toWire(),
	}
	if _, err := certs.StartChallenge(ctx, startReq); err != nil {
		var ce *console.ConsoleError
		if errors.As(err, &ce) && ce.Code == console.ErrCodeACMEChallengeInProgress {
			// Recover from an abandoned challenge: cancel it and start fresh once.
			if cerr := certs.CancelChallenge(ctx); cerr != nil {
				return nil, fmt.Errorf("a challenge was already in progress and could not be cancelled: %w", cerr)
			}
			if _, err = certs.StartChallenge(ctx, startReq); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	issued, err := certs.CompleteChallenge(ctx)
	if err != nil {
		// Best-effort cleanup so a failed issuance doesn't wedge the singleton.
		_ = certs.CancelChallenge(ctx)
		return nil, err
	}
	return issued, nil
}

func (r *consoleCertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state consoleCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cert, err := r.client.Console().Certificates().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read console certificate", err.Error())
		return
	}
	state.fromCertificate(cert)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *consoleCertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config consoleCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	certs := r.client.Console().Certificates()
	var cert *console.Certificate
	var err error

	// The only in-place content change is an uploaded cert whose PEM (stored,
	// diffed) changed → re-upload with the current private key from config.
	// letsEncrypt has no content update path (domain/source/name are
	// RequiresReplace; credentials are write-only and undiffed — rotate via
	// -replace).
	if plan.Source.ValueString() == sourceUpload && !plan.Certificate.Equal(state.Certificate) {
		cert, err = certs.Upload(ctx, console.UploadRequest{
			Name: plan.Name.ValueString(),
			Cert: strOrEmpty(plan.Certificate),
			Key:  strOrEmpty(config.PrivateKey),
		})
		if err != nil {
			addConsoleCertError(&resp.Diagnostics, "re-upload certificate", err)
			return
		}
	} else {
		cert, err = certs.Get(ctx, state.ID.ValueString())
		if err != nil {
			addConsoleCertError(&resp.Diagnostics, "read certificate", err)
			return
		}
	}

	// Reconcile the desired active (serving) state — e.g. active toggled, or a
	// re-upload landed inactive.
	cert, err = ensureActive(ctx, certs, cert, plan.Active.ValueBool())
	if err != nil {
		addConsoleCertError(&resp.Diagnostics, "activate certificate", err)
		return
	}

	plan.fromCertificate(cert)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *consoleCertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state consoleCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Delete removes the certificate; when it was the active one the console
	// reverts to its self-signed default (invariant 4: never left without a usable
	// cert). We delete rather than Revoke — revocation is a permanent, rate-limited
	// CA action and isn't needed to stop serving the cert.
	if err := r.client.Console().Certificates().Delete(ctx, state.ID.ValueString()); err != nil {
		addConsoleCertError(&resp.Diagnostics, "delete certificate", err)
	}
}

func (r *consoleCertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// toWire maps the write-only credential model onto the request union. A manual
// provider carries no credentials and is sent as an absent dnsProvider.
func (m *dnsProviderModel) toWire() *console.DNSProvider {
	if m == nil || m.Name.ValueString() == console.ProviderManual {
		return nil
	}
	return &console.DNSProvider{
		Provider:          m.Name.ValueString(),
		APIToken:          strOrEmpty(m.APIToken),
		Token:             strOrEmpty(m.Token),
		APIKey:            strOrEmpty(m.APIKey),
		APISecret:         strOrEmpty(m.APISecret),
		SecretKey:         strOrEmpty(m.SecretKey),
		AccessKeyID:       strOrEmpty(m.AccessKeyID),
		SecretAccessKey:   strOrEmpty(m.SecretAccessKey),
		Region:            strOrEmpty(m.Region),
		ApplicationKey:    strOrEmpty(m.ApplicationKey),
		ApplicationSecret: strOrEmpty(m.ApplicationSecret),
		ConsumerKey:       strOrEmpty(m.ConsumerKey),
		Endpoint:          strOrEmpty(m.Endpoint),
	}
}

// fromCertificate refreshes the computed observability fields (and reconciles
// name/source/domain so an imported certificate reaches a clean plan). It never
// touches the write-only inputs (dns_provider/private_key) nor the stored
// certificate PEM.
func (m *consoleCertificateResourceModel) fromCertificate(c *console.Certificate) {
	m.ID = types.StringValue(c.ID)
	if c.Name != "" {
		m.Name = types.StringValue(c.Name)
	}
	switch c.Source {
	case "uploaded":
		m.Source = types.StringValue(sourceUpload)
	case "lets_encrypt":
		m.Source = types.StringValue(sourceLetsEncrypt)
	}
	// Recover the domain from the subject CN on import (empty state), without
	// churning a managed Let's Encrypt resource that already has it configured.
	if m.Source.ValueString() != sourceUpload && (m.Domain.IsNull() || m.Domain.ValueString() == "") && c.Subject.CN != "" {
		m.Domain = types.StringValue(c.Subject.CN)
	}
	m.Active = types.BoolValue(c.Active)
	m.SerialNumber = types.StringValue(c.SerialNumber)
	m.Fingerprint = types.StringValue(c.Fingerprint)
	m.SubjectCommonName = types.StringValue(c.Subject.CN)
	m.Issuer = types.StringValue(c.Issuer.CN)
	m.ValidFrom = types.StringValue(c.ValidFrom)
	m.ValidTo = types.StringValue(c.ValidTo)
	m.CertVersion = types.Int64Value(int64(c.Version))
	if c.ACMERenewError != nil {
		m.ACMERenewError = types.StringValue(c.ACMERenewError.Code)
	} else {
		m.ACMERenewError = types.StringValue("")
	}
}

// addConsoleCertError maps a typed console error code to an actionable
// diagnostic (invariant 3), falling back to the raw message.
func addConsoleCertError(diags *diag.Diagnostics, action string, err error) {
	var ce *console.ConsoleError
	if errors.As(err, &ce) {
		switch ce.Code {
		case console.ErrCodeCallDNSProviderAPIFailed:
			diags.AddError("DNS provider rejected the request",
				"The console could not use the DNS provider's API to place the ACME challenge record. Verify the "+
					"credentials and that they grant edit access to the zone for this domain. ("+ce.Error()+")")
			return
		case console.ErrCodeACMEChallengeInProgress:
			diags.AddError("An ACME challenge is already in progress",
				"Only one console certificate challenge can run at a time. Wait for the in-flight challenge to finish "+
					"(or cancel it in the UI), then retry. ("+ce.Error()+")")
			return
		case console.ErrCodeACMEChallengeValidationFailed, console.ErrCodeACMEChallengeIssueFailed:
			diags.AddError("ACME challenge failed",
				"Let's Encrypt could not validate the domain. Confirm the domain resolves and (for manual DNS) that the "+
					"_acme-challenge TXT record is published. ("+ce.Error()+")")
			return
		case console.ErrCodeDNSRecordNotPropagated:
			diags.AddError("DNS challenge record did not propagate in time",
				"The _acme-challenge TXT record was not visible before the timeout. For manual DNS, publish it and "+
					"re-run; for an automated provider, verify the credentials can edit the zone. ("+ce.Error()+")")
			return
		case console.ErrCodeUserCertificateDuplicate:
			diags.AddError("A certificate with this name already exists",
				"Choose a different `name`, or remove the existing console certificate first. ("+ce.Error()+")")
			return
		case console.ErrCodeUserCertificateKeyMismatch:
			diags.AddError("Certificate and private key do not match",
				"The uploaded `certificate` and `private_key` are not a matching pair. ("+ce.Error()+")")
			return
		case console.ErrCodeUserCertificateUnsupportedType,
			console.ErrCodeUserCertificateInvalidCN,
			console.ErrCodeUserCertificateExpired:
			diags.AddError("Uploaded certificate rejected",
				"The console rejected the uploaded certificate (unsupported type, invalid common name, or already "+
					"expired). Check the PEM material. ("+ce.Error()+")")
			return
		}
		diags.AddError("Failed to "+action, ce.Error())
		return
	}
	diags.AddError("Failed to "+action, err.Error())
}
