package provider

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/datasources"
	"github.com/recolabs/terraform-provider-reco/internal/resources"
)

var (
	_ provider.Provider              = &recoProvider{}
	_ provider.ProviderWithFunctions = &recoProvider{}
)

type recoProvider struct {
	version string
}

type recoProviderModel struct {
	APIKey  types.String `tfsdk:"api_key"`
	BaseURL types.String `tfsdk:"base_url"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &recoProvider{version: version}
	}
}

func (p *recoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "reco"
	resp.Version = p.version
}

func (p *recoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Reco configuration as code. Authenticate with a Reco API key and your tenant base URL, " +
			"either in the provider block or through the RECO_API_KEY and RECO_BASE_URL environment variables.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "Reco API key. Create one in the Reco platform under Settings → API Keys. May also be set via the RECO_API_KEY environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"base_url": schema.StringAttribute{
				Description: "Base URL of your Reco tenant, the same URL you use to sign in to the Reco platform. Must use https. May also be set via the RECO_BASE_URL environment variable.",
				Optional:    true,
			},
		},
	}
}

func (p *recoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config recoProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := os.Getenv("RECO_API_KEY")
	if !config.APIKey.IsNull() && !config.APIKey.IsUnknown() {
		apiKey = config.APIKey.ValueString()
	}
	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing API Key",
			"Set the api_key provider attribute or the RECO_API_KEY environment variable.",
		)
	}

	baseURL := os.Getenv("RECO_BASE_URL")
	if !config.BaseURL.IsNull() && !config.BaseURL.IsUnknown() {
		baseURL = config.BaseURL.ValueString()
	}
	if baseURL == "" {
		resp.Diagnostics.AddError(
			"Missing Base URL",
			"Set the base_url provider attribute or the RECO_BASE_URL environment variable.",
		)
	} else if !strings.HasPrefix(baseURL, "https://") {
		resp.Diagnostics.AddError(
			"Insecure Base URL",
			"base_url must use the https:// scheme to protect the API key in transit.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	c := client.New(baseURL, apiKey, p.version)
	if ps := os.Getenv("RECO_TEST_PAGE_SIZE"); ps != "" {
		if n, err := strconv.ParseInt(ps, 10, 64); err == nil && n > 0 {
			c.WithPageSize(n)
		}
	}
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *recoProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewUserResource,
		resources.NewRoleResource,
		resources.NewPostureCheckResource,
		resources.NewThreatDetectionPolicyResource,
		resources.NewApiKeyResource,
	}
}

func (p *recoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewPoliciesDataSource,
		datasources.NewRolesDataSource,
		datasources.NewIntegrationsDataSource,
		datasources.NewUsersDataSource,
		datasources.NewPostureChecksDataSource,
		datasources.NewApiKeysDataSource,
	}
}

func (p *recoProvider) Functions(_ context.Context) []func() function.Function {
	return nil
}
