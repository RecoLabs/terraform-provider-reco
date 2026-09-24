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
	_ datasource.DataSource              = &rolesDataSource{}
	_ datasource.DataSourceWithConfigure = &rolesDataSource{}
)

type rolesDataSource struct {
	client *client.Client
}

func NewRolesDataSource() datasource.DataSource {
	return &rolesDataSource{}
}

type rolesDataSourceModel struct {
	Roles []roleDSModel `tfsdk:"roles"`
}

type roleDSModel struct {
	Name        types.String         `tfsdk:"name"`
	Description types.String         `tfsdk:"description"`
	Permissions types.List           `tfsdk:"permissions"`
	Resources   []roleDSResourcePerm `tfsdk:"resources"`
	Type        types.String         `tfsdk:"type"`
	CreatedAt   types.String         `tfsdk:"created_at"`
}

type roleDSResourcePerm struct {
	ResourceName types.String `tfsdk:"resource_name"`
	Resource     types.String `tfsdk:"resource"`
	Permission   types.String `tfsdk:"permission"`
}

func (d *rolesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_roles"
}

func (d *rolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all RBAC roles (custom and system).",
		Attributes: map[string]schema.Attribute{
			"roles": schema.ListNestedAttribute{
				Description: "List of roles.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						attrName:      schema.StringAttribute{Computed: true, Description: "Role name."},
						"description": schema.StringAttribute{Computed: true, Description: "Role description."},
						"permissions": schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Permissions granted by this role."},
						"resources": schema.ListNestedAttribute{
							Computed:    true,
							Description: "Fine-grained resource-level permissions attached to this role.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"resource_name": schema.StringAttribute{Computed: true, Description: "Display name of the resource."},
									"resource":      schema.StringAttribute{Computed: true, Description: "Resource identifier."},
									"permission":    schema.StringAttribute{Computed: true, Description: "Permission granted on the resource."},
								},
							},
						},
						"type":       schema.StringAttribute{Computed: true, Description: "Role type (CUSTOM, SYSTEM, etc.)."},
						"created_at": schema.StringAttribute{Computed: true, Description: creationTimestampDescription},
					},
				},
			},
		},
	}
}

func (d *rolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &d.client, &resp.Diagnostics)
}

func (d *rolesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	roles, err := d.client.ListAllRoles(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing roles", err.Error())
		return
	}

	models := make([]roleDSModel, len(roles))
	for i, r := range roles {
		resources := make([]roleDSResourcePerm, len(r.Resources))
		for j, rp := range r.Resources {
			resources[j] = roleDSResourcePerm{
				ResourceName: types.StringValue(rp.ResourceName),
				Resource:     types.StringValue(rp.Resource),
				Permission:   types.StringValue(rp.Permission),
			}
		}
		models[i] = roleDSModel{
			Name:        types.StringValue(r.Name),
			Description: types.StringValue(r.Description),
			Type:        types.StringValue(r.Type),
			Permissions: tfutil.StringSliceToList(r.Permissions),
			Resources:   resources,
			CreatedAt:   tfutil.StringPtrValue(r.CreatedAt),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &rolesDataSourceModel{Roles: models})...)
}
