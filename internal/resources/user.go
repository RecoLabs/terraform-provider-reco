package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	tfutil "github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

var (
	_ resource.Resource                = &userResource{}
	_ resource.ResourceWithImportState = &userResource{}
	_ resource.ResourceWithConfigure   = &userResource{}
)

type userResource struct {
	client *client.Client
}

func NewUserResource() resource.Resource {
	return &userResource{}
}

type userResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	UserID                 types.String `tfsdk:"user_id"`
	EmailAddress           types.String `tfsdk:"email_address"`
	Name                   types.String `tfsdk:"name"`
	UserRoles              types.List   `tfsdk:"user_roles"`
	Segments               types.List   `tfsdk:"segments"`
	Expiration             types.String `tfsdk:"expiration"`
	IsBypassSSOEnforcement types.Bool   `tfsdk:"is_bypass_sso_enforcement"`
	EnablementStatus       types.String `tfsdk:"enablement_status"`
	AuthMethods            types.List   `tfsdk:"auth_methods"`
	ProfilePictureURL      types.String `tfsdk:"profile_picture_url"`
	CreationTime           types.String `tfsdk:"creation_time"`
	LastLoginTime          types.String `tfsdk:"last_login_time"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Reco user. The stable identifier is user_id (set after create, never changes). " +
			"Changing email_address updates the user in place — it does not force a replacement. " +
			"Import accepts either the email address or the user_id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Terraform resource identifier. Equal to user_id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.StringAttribute{
				Description: "Reco user ID. Stable identifier assigned on creation.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email_address": schema.StringAttribute{
				Description: "User email address. Changing this updates the user in place.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Display name of the user.",
				Required:    true,
			},
			"user_roles": schema.ListAttribute{
				Description: "Role names assigned to the user. A user must have at least one role.",
				Required:    true,
				ElementType: types.StringType,
				Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
			},
			"segments": schema.ListAttribute{
				Description: "List of segment names the user belongs to.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"expiration": schema.StringAttribute{
				Description: "User expiration timestamp in RFC3339 format. Omit or set to null for no expiration.",
				Optional:    true,
			},
			"is_bypass_sso_enforcement": schema.BoolAttribute{
				Description: "When true, the user can sign in without SSO even when SSO enforcement is enabled.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"enablement_status": schema.StringAttribute{
				Description: "User enablement status: enabled, disabled, or invited (invited is read-only).",
				Optional:    true,
				Computed:    true,
			},
			"auth_methods": schema.ListAttribute{
				Description: "Authentication methods active for this user (read-only).",
				Computed:    true,
				ElementType: types.StringType,
			},
			"profile_picture_url": schema.StringAttribute{
				Description: "Profile picture URL (read-only).",
				Computed:    true,
			},
			"creation_time": schema.StringAttribute{
				Description: "User creation timestamp (read-only).",
				Computed:    true,
			},
			"last_login_time": schema.StringAttribute{
				Description: "Last login timestamp (read-only).",
				Computed:    true,
			},
		},
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &r.client, &resp.Diagnostics)
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user := modelToRecoUser(&plan)
	created, err := r.client.CreateUser(ctx, user)
	if err != nil {
		resp.Diagnostics.AddError("Error creating user", err.Error())
		return
	}

	if created.UserID != "" {
		plan.UserID = types.StringValue(created.UserID)
		plan.ID = plan.UserID
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		resp.Diagnostics.AddWarning(
			"User created but ID not returned",
			"The API did not return a user_id in the create response. "+
				"If the follow-up read fails, use `terraform import` to recover.",
		)
	}

	var full *client.RecoUser
	if plan.UserID.ValueString() != "" {
		full, err = r.client.GetUserByID(ctx, plan.UserID.ValueString())
	} else {
		full, err = r.client.GetUserByEmail(ctx, plan.EmailAddress.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading created user", err.Error())
		return
	}

	recoUserToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := r.client.GetUserByID(ctx, state.UserID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}

	recoUserToModel(user, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userResourceModel
	var state userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.UserID = state.UserID
	plan.ID = state.ID

	updateReq := client.UpdateRecoUserRequest{
		User:             *modelToRecoUser(&plan),
		EnablementStatus: plan.EnablementStatus.ValueString(),
	}
	if err := r.client.UpdateUser(ctx, &updateReq); err != nil {
		resp.Diagnostics.AddError("Error updating user", err.Error())
		return
	}

	updated, err := r.client.GetUserByID(ctx, state.UserID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated user", err.Error())
		return
	}

	recoUserToModel(updated, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteUser(ctx, state.UserID.ValueString()); err != nil {
		if !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Error deleting user", err.Error())
		}
	}
}

func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	var user *client.RecoUser
	var err error

	if strings.Contains(id, "@") {
		user, err = r.client.GetUserByEmail(ctx, id)
	} else {
		user, err = r.client.GetUserByID(ctx, id)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error importing user", fmt.Sprintf("cannot find user %q: %s", id, err))
		return
	}

	var state userResourceModel
	recoUserToModel(user, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func modelToRecoUser(m *userResourceModel) *client.RecoUser {
	u := &client.RecoUser{
		UserID:                 m.UserID.ValueString(),
		EmailAddress:           m.EmailAddress.ValueString(),
		Name:                   m.Name.ValueString(),
		IsBypassSSOEnforcement: m.IsBypassSSOEnforcement.ValueBool(),
		EnablementStatus:       m.EnablementStatus.ValueString(),
	}
	if !m.Expiration.IsNull() && !m.Expiration.IsUnknown() {
		s := m.Expiration.ValueString()
		u.Expiration = &s
	}
	u.UserRoles = tfutil.ListToStringSlice(m.UserRoles)
	if !m.Segments.IsNull() && !m.Segments.IsUnknown() {
		u.Segments = tfutil.ListToStringSlice(m.Segments)
	} else {
		u.Segments = []string{}
	}
	return u
}

func recoUserToModel(u *client.RecoUser, m *userResourceModel) {
	m.ID = types.StringValue(u.UserID)
	m.UserID = types.StringValue(u.UserID)
	m.EmailAddress = types.StringValue(u.EmailAddress)
	m.Name = types.StringValue(u.Name)
	m.IsBypassSSOEnforcement = types.BoolValue(u.IsBypassSSOEnforcement)
	m.EnablementStatus = types.StringValue(u.EnablementStatus)
	m.ProfilePictureURL = types.StringValue(u.ProfilePictureURL)

	m.Expiration = tfutil.StringPtrValue(u.Expiration)
	m.CreationTime = tfutil.StringPtrValue(u.CreationTime)
	m.LastLoginTime = tfutil.StringPtrValue(u.LastLoginTime)

	m.UserRoles = tfutil.StringSliceToList(u.UserRoles)
	m.Segments = tfutil.StringSliceToList(u.Segments)
	m.AuthMethods = tfutil.StringSliceToList(u.AuthMethods)
}
