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
	_ datasource.DataSource              = &policiesDataSource{}
	_ datasource.DataSourceWithConfigure = &policiesDataSource{}
)

type policiesDataSource struct {
	client *client.Client
}

func NewPoliciesDataSource() datasource.DataSource {
	return &policiesDataSource{}
}

type policiesDataSourceModel struct {
	Policies []policyModel `tfsdk:"policies"`
}

type policyModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Apps       types.List   `tfsdk:"apps"`
	Severity   types.String `tfsdk:"severity"`
	Status     types.String `tfsdk:"status"`
	PolicyType types.String `tfsdk:"policy_type"`
	Tags       types.List   `tfsdk:"tags"`
	CreatedAt  types.String `tfsdk:"created_at"`
	OpenAlerts types.Int64  `tfsdk:"open_alerts"`
	AllAlerts  types.Int64  `tfsdk:"all_alerts"`
}

func (d *policiesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policies"
}

func (d *policiesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all threat detection policies. Data sources are intentionally limited to small, stable configuration objects.",
		Attributes: map[string]schema.Attribute{
			"policies": schema.ListNestedAttribute{
				Description: "List of threat detection policies.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, Description: "Policy ID."},
						attrName:      schema.StringAttribute{Computed: true, Description: "Policy name."},
						"apps":        schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Associated applications."},
						"severity":    schema.StringAttribute{Computed: true, Description: "Severity level."},
						"status":      schema.StringAttribute{Computed: true, Description: "Policy status (active/inactive)."},
						"policy_type": schema.StringAttribute{Computed: true, Description: "Policy type."},
						"tags":        schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Tags."},
						"created_at":  schema.StringAttribute{Computed: true, Description: creationTimestampDescription},
						"open_alerts": schema.Int64Attribute{Computed: true, Description: "Number of open alerts."},
						"all_alerts":  schema.Int64Attribute{Computed: true, Description: "Total alert count."},
					},
				},
			},
		},
	}
}

func (d *policiesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &d.client, &resp.Diagnostics)
}

func (d *policiesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	policies, err := d.client.ListAllPolicies(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing policies", err.Error())
		return
	}

	models := make([]policyModel, len(policies))
	for i := range policies {
		p := &policies[i]
		models[i] = policyModel{
			ID:         types.StringValue(p.ID),
			Name:       types.StringValue(p.Name),
			Severity:   types.StringValue(p.Severity),
			Status:     types.StringValue(p.Status),
			PolicyType: types.StringValue(p.PolicyType),
			OpenAlerts: types.Int64Value(int64(p.OpenAlerts)),
			AllAlerts:  types.Int64Value(int64(p.AllAlerts)),
			Apps:       tfutil.StringSliceToList(p.Apps),
			Tags:       tfutil.StringSliceToList(p.Tags),
		}
		models[i].CreatedAt = tfutil.StringPtrValue(p.CreatedAt)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &policiesDataSourceModel{Policies: models})...)
}
