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
	_ datasource.DataSource              = &usersDataSource{}
	_ datasource.DataSourceWithConfigure = &usersDataSource{}
)

type usersDataSource struct {
	client *client.Client
}

func NewUsersDataSource() datasource.DataSource {
	return &usersDataSource{}
}

type usersDataSourceModel struct {
	Users []userDSModel `tfsdk:"users"`
}

type userDSModel struct {
	UserID                 types.String `tfsdk:"user_id"`
	EmailAddress           types.String `tfsdk:"email_address"`
	Name                   types.String `tfsdk:"name"`
	UserRoles              types.List   `tfsdk:"user_roles"`
	Segments               types.List   `tfsdk:"segments"`
	EnablementStatus       types.String `tfsdk:"enablement_status"`
	IsBypassSSOEnforcement types.Bool   `tfsdk:"is_bypass_sso_enforcement"`
	AuthMethods            types.List   `tfsdk:"auth_methods"`
	CreationTime           types.String `tfsdk:"creation_time"`
	LastLoginTime          types.String `tfsdk:"last_login_time"`
}

func (d *usersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *usersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all Reco users. Use this data source to reference user_id values in reco_user resources.",
		Attributes: map[string]schema.Attribute{
			"users": schema.ListNestedAttribute{
				Description: "List of users.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"user_id":                   schema.StringAttribute{Computed: true, Description: "Stable Reco user ID."},
						"email_address":             schema.StringAttribute{Computed: true, Description: "Email address."},
						attrName:                    schema.StringAttribute{Computed: true, Description: "Display name."},
						"user_roles":                schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Assigned roles."},
						"segments":                  schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Assigned segments."},
						"enablement_status":         schema.StringAttribute{Computed: true, Description: "Enablement status."},
						"is_bypass_sso_enforcement": schema.BoolAttribute{Computed: true, Description: "SSO bypass flag."},
						"auth_methods":              schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Active authentication methods."},
						"creation_time":             schema.StringAttribute{Computed: true, Description: creationTimestampDescription},
						"last_login_time":           schema.StringAttribute{Computed: true, Description: "Last login timestamp."},
					},
				},
			},
		},
	}
}

func (d *usersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &d.client, &resp.Diagnostics)
}

func (d *usersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	users, err := d.client.ListAllUsers(ctx, "")
	if err != nil {
		resp.Diagnostics.AddError("Error listing users", err.Error())
		return
	}

	models := make([]userDSModel, len(users))
	for i := range users {
		u := &users[i]
		models[i] = userDSModel{
			UserID:                 types.StringValue(u.UserID),
			EmailAddress:           types.StringValue(u.EmailAddress),
			Name:                   types.StringValue(u.Name),
			EnablementStatus:       types.StringValue(u.EnablementStatus),
			IsBypassSSOEnforcement: types.BoolValue(u.IsBypassSSOEnforcement),
			UserRoles:              tfutil.StringSliceToList(u.UserRoles),
			Segments:               tfutil.StringSliceToList(u.Segments),
			AuthMethods:            tfutil.StringSliceToList(u.AuthMethods),
		}
		models[i].CreationTime = tfutil.StringPtrValue(u.CreationTime)
		models[i].LastLoginTime = tfutil.StringPtrValue(u.LastLoginTime)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &usersDataSourceModel{Users: models})...)
}
