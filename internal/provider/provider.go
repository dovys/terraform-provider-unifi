// Package provider wires the UniFi OS Terraform provider: configuration schema,
// credential resolution (config + env), and the Configure-time client build +
// capability probe. Resources and data sources are registered per phase.
package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// Ensure UnifiProvider satisfies the framework provider interface.
var _ provider.Provider = (*UnifiProvider)(nil)

// UnifiProvider is the provider implementation.
type UnifiProvider struct {
	// version is set at build time and surfaced to the registry.
	version string
}

// New returns a provider factory for the given build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &UnifiProvider{version: version}
	}
}

// providerModel mirrors the provider configuration block.
type providerModel struct {
	Host          types.String `tfsdk:"host"`
	APIKey        types.String `tfsdk:"api_key"`
	Username      types.String `tfsdk:"username"`
	Password      types.String `tfsdk:"password"`
	Site          types.String `tfsdk:"site"`
	AllowInsecure types.Bool   `tfsdk:"allow_insecure"`
	Timeout       types.String `tfsdk:"timeout"`
	MaxRetries    types.Int64  `tfsdk:"max_retries"`
}

func (p *UnifiProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "unifi"
	resp.Version = p.version
}

func (p *UnifiProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a UniFi OS console (Network app today; Protect later) as code. " +
			"Connects to a single console over its local API surfaces. A local UniFi OS admin " +
			"account is effectively required; an Integration API key unlocks the official API where " +
			"firmware supports it.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Console base URL or host, e.g. `https://192.0.2.1` or `unifi.example.com`. " +
					"A bare host is assumed `https://`. Env: `UNIFI_HOST`.",
			},
			"api_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "Integration API key (Network → Settings → Control Plane → Integrations). " +
					"Console-wide and owner-level — treat as admin-equivalent. Env: `UNIFI_API_KEY`.",
			},
			"username": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Local UniFi OS admin username (not a cloud/SSO account). " +
					"Required together with `password`. Env: `UNIFI_USERNAME`.",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Password for `username`. Env: `UNIFI_PASSWORD`.",
			},
			"site": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Site slug to manage (the internal reference, e.g. `default`). Defaults to `default`. Env: `UNIFI_SITE`.",
			},
			"allow_insecure": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "Skip TLS certificate verification. UniFi consoles ship self-signed certs on " +
					"local IPs, so this is commonly needed against a LAN console. Defaults to `false`. Env: `UNIFI_INSECURE`.",
			},
			"timeout": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Per-request timeout as a Go duration (e.g. `60s`, `2m`). Defaults to `60s`. Env: `UNIFI_TIMEOUT`.",
			},
			"max_retries": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Number of times to retry a request that returns HTTP 429 (rate limited). Defaults to `3`. Env: `UNIFI_MAX_RETRIES`.",
			},
		},
	}
}

func (p *UnifiProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Provider config must be known at configure time (not derived during apply).
	for name, unknown := range map[string]bool{
		"host":     data.Host.IsUnknown(),
		"api_key":  data.APIKey.IsUnknown(),
		"username": data.Username.IsUnknown(),
		"password": data.Password.IsUnknown(),
	} {
		if unknown {
			resp.Diagnostics.AddAttributeError(path.Root(name),
				"Unknown provider configuration value",
				fmt.Sprintf("The %q value is unknown at configure time. Set it to a static value or a known input.", name))
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	host := stringWithEnv(data.Host, "UNIFI_HOST")
	apiKey := stringWithEnv(data.APIKey, "UNIFI_API_KEY")
	username := stringWithEnv(data.Username, "UNIFI_USERNAME")
	password := stringWithEnv(data.Password, "UNIFI_PASSWORD")
	site := stringWithEnvDefault(data.Site, "UNIFI_SITE", "default")
	insecure := boolWithEnv(data.AllowInsecure, "UNIFI_INSECURE")

	timeout := 60 * time.Second
	if v := stringWithEnv(data.Timeout, "UNIFI_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("timeout"),
				"Invalid timeout",
				fmt.Sprintf("%q is not a valid Go duration (e.g. \"60s\", \"2m\"): %s", v, err))
			return
		}
		timeout = d
	}

	maxRetries := 3
	if !data.MaxRetries.IsNull() {
		maxRetries = int(data.MaxRetries.ValueInt64())
	} else if v := strings.TrimSpace(os.Getenv("UNIFI_MAX_RETRIES")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("max_retries"),
				"Invalid max_retries", fmt.Sprintf("UNIFI_MAX_RETRIES=%q is not an integer: %s", v, err))
			return
		}
		maxRetries = n
	}

	// Validate the resolved configuration before any network call.
	if host == "" {
		resp.Diagnostics.AddAttributeError(path.Root("host"),
			"Missing host", "Set `host` or the UNIFI_HOST environment variable (e.g. https://192.0.2.1).")
	}
	if (username == "") != (password == "") {
		resp.Diagnostics.AddError("Incomplete credentials",
			"`username` and `password` must be set together (a local UniFi OS admin account).")
	}
	if username == "" && password == "" && apiKey == "" {
		resp.Diagnostics.AddError("Missing credentials",
			"Provide `username`+`password` (a local UniFi OS admin) and/or `api_key`.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := unifi.NewClient(ctx, unifi.Config{
		Host:          host,
		APIKey:        apiKey,
		Username:      username,
		Password:      password,
		Site:          site,
		AllowInsecure: insecure,
		Timeout:       timeout,
		MaxRetries:    maxRetries,
	})
	if err != nil {
		summary, detail := describeClientError(err)
		resp.Diagnostics.AddError(summary, detail)
		return
	}

	caps := client.Capabilities()
	tflog.Info(ctx, "configured UniFi provider", map[string]any{
		"site":                  client.Site(),
		"site_id":               client.SiteID(),
		"version":               caps.VersionRaw,
		"integration_writes":    caps.Flags.IntegrationWrites,
		"integration_available": client.Router().IntegrationAvailable(),
	})

	resp.ResourceData = client
	resp.DataSourceData = client
}

// Resources registers the provider's managed resources. Populated per phase.
func (p *UnifiProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

// DataSources registers the provider's data sources. Populated per phase.
func (p *UnifiProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

// describeClientError maps a NewClient error to a user-facing diagnostic
// (summary, detail), turning low-level failures into actionable guidance.
func describeClientError(err error) (string, string) {
	var twoFA *transport.TwoFactorError
	if errors.As(err, &twoFA) {
		return "UniFi login requires 2FA",
			"This account is a cloud/SSO account or has MFA enabled. The provider needs a local " +
				"UniFi OS admin account: Settings → Admins → add a local-only admin, then set " +
				"`username`/`password` to it."
	}
	var authErr *transport.AuthError
	if errors.As(err, &authErr) {
		return "UniFi login failed",
			err.Error() + "\n\nVerify `host`, `username`, and `password`. The account must be a local UniFi OS admin."
	}
	var capErr *unifi.CapabilityError
	if errors.As(err, &capErr) {
		return "UniFi capability unavailable", err.Error()
	}
	var cfgErr *unifi.ConfigError
	if errors.As(err, &cfgErr) {
		return "UniFi provider configuration error", err.Error()
	}
	return "Failed to configure the UniFi client", err.Error()
}

func stringWithEnv(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}

func stringWithEnvDefault(v types.String, env, def string) string {
	if s := stringWithEnv(v, env); s != "" {
		return s
	}
	return def
}

func boolWithEnv(v types.Bool, env string) bool {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueBool()
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(env))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
