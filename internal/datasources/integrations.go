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
	_ datasource.DataSource              = &integrationsDataSource{}
	_ datasource.DataSourceWithConfigure = &integrationsDataSource{}
)

type integrationsDataSource struct {
	client *client.Client
}

func NewIntegrationsDataSource() datasource.DataSource {
	return &integrationsDataSource{}
}

type integrationsDataSourceModel struct {
	Integrations []integrationModel `tfsdk:"integrations"`
}

type integrationModel struct {
	InstanceID      types.String `tfsdk:"instance_id"`
	App             types.String `tfsdk:"app"`
	Status          types.String `tfsdk:"status"`
	HasNewEndpoints types.Bool   `tfsdk:"has_new_endpoints"`
	CreatedOn       types.String `tfsdk:"created_on"`
	LastUpdatedOn   types.String `tfsdk:"last_updated_on"`
	LastUpdatedBy   types.String `tfsdk:"last_updated_by"`
	SeverityConfig  types.String `tfsdk:"severity_config"`
	ErrorDetails    types.String `tfsdk:"error_details"`
}

func (d *integrationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integrations"
}

func (d *integrationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all connected integrations.",
		Attributes: map[string]schema.Attribute{
			"integrations": schema.ListNestedAttribute{
				Description: "List of integrations.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"instance_id":       schema.StringAttribute{Computed: true, Description: "Integration instance ID."},
						"app":               schema.StringAttribute{Computed: true, Description: "Data source / application identifier."},
						"status":            schema.StringAttribute{Computed: true, Description: "Integration status."},
						"has_new_endpoints": schema.BoolAttribute{Computed: true, Description: "Whether new endpoints are available."},
						"created_on":        schema.StringAttribute{Computed: true, Description: creationTimestampDescription},
						"last_updated_on":   schema.StringAttribute{Computed: true, Description: "Last update timestamp."},
						"last_updated_by":   schema.StringAttribute{Computed: true, Description: "User who last updated the integration."},
						"severity_config":   schema.StringAttribute{Computed: true, Description: "Severity configuration."},
						"error_details":     schema.StringAttribute{Computed: true, Description: "Error details if integration is unhealthy."},
					},
				},
			},
		},
	}
}

func (d *integrationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &d.client, &resp.Diagnostics)
}

func (d *integrationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	integrations, err := d.client.ListAllIntegrations(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing integrations", err.Error())
		return
	}

	models := make([]integrationModel, len(integrations))
	for i := range integrations {
		intg := &integrations[i]
		models[i] = integrationModel{
			InstanceID:      types.StringValue(intg.Instance.ID),
			App:             types.StringValue(intg.App),
			Status:          types.StringValue(intg.Status),
			HasNewEndpoints: types.BoolValue(intg.HasNewEndpoints),
			LastUpdatedBy:   types.StringValue(intg.LastUpdatedBy),
			SeverityConfig:  types.StringValue(intg.SeverityConfig),
			ErrorDetails:    types.StringValue(intg.ErrorDetails),
		}
		models[i].CreatedOn = tfutil.StringPtrValue(intg.CreatedOn)
		models[i].LastUpdatedOn = tfutil.StringPtrValue(intg.LastUpdatedOn)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &integrationsDataSourceModel{Integrations: models})...)
}
