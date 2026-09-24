package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

var (
	_ datasource.DataSource              = &apiKeysDataSource{}
	_ datasource.DataSourceWithConfigure = &apiKeysDataSource{}
)

type apiKeysDataSource struct {
	client *client.Client
}

func NewApiKeysDataSource() datasource.DataSource {
	return &apiKeysDataSource{}
}

type apiKeysDataSourceModel struct {
	Keys []apiKeyModel `tfsdk:"api_keys"`
}

type apiKeyModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Role         types.String `tfsdk:"role"`
	CreatedBy    types.String `tfsdk:"created_by"`
	State        types.String `tfsdk:"state"`
	PermittedIps types.List   `tfsdk:"permitted_ips"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

func (d *apiKeysDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_keys"
}

func (d *apiKeysDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all API keys for the tenant. Does not include secrets.",
		Attributes: map[string]schema.Attribute{
			"api_keys": schema.ListNestedAttribute{
				Description: "List of API keys.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":            schema.StringAttribute{Computed: true, Description: "Key ID."},
						attrName:        schema.StringAttribute{Computed: true, Description: "Key name."},
						"role":          schema.StringAttribute{Computed: true, Description: "Assigned role name."},
						"created_by":    schema.StringAttribute{Computed: true, Description: "Email of the creator."},
						"state":         schema.StringAttribute{Computed: true, Description: "Key state."},
						"permitted_ips": schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "IP allowlist."},
						attrCreatedAt:   schema.StringAttribute{Computed: true, Description: creationTimestampDescription},
					},
				},
			},
		},
	}
}

func (d *apiKeysDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &d.client, &resp.Diagnostics)
}

func (d *apiKeysDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	keys, err := d.client.ListAllApiKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing API keys", err.Error())
		return
	}

	models := make([]apiKeyModel, len(keys))
	for i, k := range keys {
		models[i] = apiKeyModel{
			ID:           types.StringValue(k.ID),
			Name:         types.StringValue(k.Name),
			Role:         types.StringValue(k.Role),
			CreatedBy:    types.StringValue(k.CreatedBy),
			State:        types.StringValue(k.State),
			CreatedAt:    tfutil.StringPtrValue(k.CreatedAt),
			PermittedIps: tfutil.StringSliceToList(k.PermittedIps),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &apiKeysDataSourceModel{Keys: models})...)
}
