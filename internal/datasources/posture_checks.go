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
	_ datasource.DataSource              = &postureChecksDataSource{}
	_ datasource.DataSourceWithConfigure = &postureChecksDataSource{}
)

type postureChecksDataSource struct {
	client *client.Client
}

func NewPostureChecksDataSource() datasource.DataSource {
	return &postureChecksDataSource{}
}

type postureChecksDataSourceModel struct {
	PostureChecks []postureCheckModel `tfsdk:"posture_checks"`
}

type postureCheckModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Tags        types.List   `tfsdk:"tags"`
	Apps        types.List   `tfsdk:"apps"`
	Severity    types.String `tfsdk:"severity"`
	PolicyType  types.String `tfsdk:"policy_type"`
	AppSource   types.String `tfsdk:"app_source"`
}

func (d *postureChecksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_posture_checks"
}

func (d *postureChecksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all posture check definitions.",
		Attributes: map[string]schema.Attribute{
			"posture_checks": schema.ListNestedAttribute{
				Description: "List of posture checks.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, Description: "Posture check ID."},
						attrName:      schema.StringAttribute{Computed: true, Description: "Posture check display name."},
						"description": schema.StringAttribute{Computed: true, Description: "Detailed description."},
						"tags":        schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Tags."},
						"apps":        schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Applicable integrations."},
						"severity":    schema.StringAttribute{Computed: true, Description: "Severity level."},
						"policy_type": schema.StringAttribute{Computed: true, Description: "Policy type identifier."},
						"app_source":  schema.StringAttribute{Computed: true, Description: "Source application identifier."},
					},
				},
			},
		},
	}
}

func (d *postureChecksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &d.client, &resp.Diagnostics)
}

func (d *postureChecksDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	checks, err := d.client.ListAllPostureChecks(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing posture checks", err.Error())
		return
	}

	models := make([]postureCheckModel, len(checks))
	for i := range checks {
		c := &checks[i]
		models[i] = postureCheckModel{
			ID:          types.StringValue(c.ID),
			Name:        types.StringValue(c.Name),
			Description: types.StringValue(c.Description),
			Severity:    types.StringValue(c.Severity),
			PolicyType:  types.StringValue(c.PolicyType),
			AppSource:   types.StringValue(c.AppSource),
			Tags:        tfutil.StringSliceToList(c.Tags),
			Apps:        tfutil.StringSliceToList(c.Apps),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &postureChecksDataSourceModel{PostureChecks: models})...)
}
