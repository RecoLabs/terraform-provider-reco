package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	tfutil "github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

var (
	_ resource.Resource                = &roleResource{}
	_ resource.ResourceWithImportState = &roleResource{}
	_ resource.ResourceWithConfigure   = &roleResource{}
)

var resourcePermAttrTypes = map[string]attr.Type{
	attrResourceName: types.StringType,
	attrResource:     types.StringType,
	attrPermission:   types.StringType,
}

type roleResource struct {
	client *client.Client
}

func NewRoleResource() resource.Resource {
	return &roleResource{}
}

type roleResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Permissions types.List   `tfsdk:"permissions"`
	Resources   types.List   `tfsdk:"resources"`
	Type        types.String `tfsdk:"type"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (r *roleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *roleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Reco custom RBAC role. The stable identifier is the role name. " +
			"Changing the name forces the resource to be destroyed and recreated.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Terraform resource identifier. Equal to the role name.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Unique role name. Changing this forces a replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "Human-readable description of the role.",
				Optional:    true,
				Computed:    true,
			},
			"permissions": schema.ListAttribute{
				Description: "List of permission strings granted by this role.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"resources": schema.ListNestedAttribute{
				Description: "Fine-grained resource-level permissions attached to this role.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						attrResourceName: schema.StringAttribute{Required: true, Description: "Display name of the resource."},
						attrResource:     schema.StringAttribute{Required: true, Description: "Resource identifier."},
						attrPermission:   schema.StringAttribute{Required: true, Description: "Permission granted on the resource."},
					},
				},
			},
			"type": schema.StringAttribute{
				Description: "Role type (read-only), e.g. USER_ROLE_TYPE_CUSTOM.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the role was created (read-only).",
				Computed:    true,
			},
		},
	}
}

func (r *roleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &r.client, &resp.Diagnostics)
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.CreateRole(ctx, modelToRecoCustomRole(&plan)); err != nil {
		resp.Diagnostics.AddError("Error creating role", err.Error())
		return
	}

	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.GetRoleByName(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created role", err.Error())
		return
	}

	recoRoleToModel(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.GetRoleByName(ctx, state.Name.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading role", err.Error())
		return
	}

	recoRoleToModel(role, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateRole(ctx, modelToRecoCustomRole(&plan), state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating role", err.Error())
		return
	}

	updated, err := r.client.GetRoleByName(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated role", err.Error())
		return
	}

	recoRoleToModel(updated, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteRole(ctx, state.Name.ValueString()); err != nil {
		if !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Error deleting role", err.Error())
		}
	}
}

func (r *roleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	role, err := r.client.GetRoleByName(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing role", fmt.Sprintf("cannot find role %q: %s", req.ID, err))
		return
	}

	var state roleResourceModel
	recoRoleToModel(role, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func modelToRecoCustomRole(m *roleResourceModel) *client.RecoCustomRole {
	resources := make([]client.ResourcePermission, 0, len(m.Resources.Elements()))
	for _, elem := range m.Resources.Elements() {
		obj, ok := elem.(types.Object)
		if !ok {
			continue
		}
		attrs := obj.Attributes()
		resources = append(resources, client.ResourcePermission{
			ResourceName: attrs[attrResourceName].(types.String).ValueString(),
			Resource:     attrs[attrResource].(types.String).ValueString(),
			Permission:   attrs[attrPermission].(types.String).ValueString(),
		})
	}
	permissions := tfutil.ListToStringSlice(m.Permissions)
	if permissions == nil {
		permissions = []string{}
	}
	return &client.RecoCustomRole{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueString(),
		Permissions: permissions,
		Resources:   resources,
	}
}

func recoRoleToModel(r *client.RecoRole, m *roleResourceModel) {
	m.ID = types.StringValue(r.Name)
	m.Name = types.StringValue(r.Name)
	m.Description = types.StringValue(r.Description)
	m.Type = types.StringValue(r.Type)
	m.Permissions = tfutil.StringSliceToList(r.Permissions)
	m.CreatedAt = tfutil.StringPtrValue(r.CreatedAt)

	elems := make([]attr.Value, len(r.Resources))
	for i, rp := range r.Resources {
		elems[i] = types.ObjectValueMust(resourcePermAttrTypes, map[string]attr.Value{
			attrResourceName: types.StringValue(rp.ResourceName),
			attrResource:     types.StringValue(rp.Resource),
			attrPermission:   types.StringValue(rp.Permission),
		})
	}
	m.Resources = types.ListValueMust(types.ObjectType{AttrTypes: resourcePermAttrTypes}, elems)
}
