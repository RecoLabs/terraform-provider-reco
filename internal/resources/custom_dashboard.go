package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	tfutil "github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

var (
	_ resource.Resource                = &customDashboardResource{}
	_ resource.ResourceWithImportState = &customDashboardResource{}
	_ resource.ResourceWithConfigure   = &customDashboardResource{}
)

type customDashboardResource struct {
	client *client.Client
}

func NewCustomDashboardResource() resource.Resource {
	return &customDashboardResource{}
}

type customDashboardResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Spec          types.String `tfsdk:"spec"`
	Visibility    types.String `tfsdk:"visibility"`
	FolderPath    types.String `tfsdk:"folder_path"`
	CreatedBy     types.String `tfsdk:"created_by"`
	CreatedByName types.String `tfsdk:"created_by_name"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func (r *customDashboardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_dashboard"
}

func (r *customDashboardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Reco custom dashboard. The Reco API cannot delete dashboards, so destroying this resource " +
			"only removes it from Terraform state; the dashboard remains in Reco and must be deleted in the Reco UI.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Dashboard ID (set after create).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			attrName: schema.StringAttribute{
				Description: "Display name (1-200 characters).",
				Required:    true,
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 200)},
			},
			"spec": schema.StringAttribute{
				Description: "Dashboard layout as a JSON object string, for example from jsonencode(). It must contain a " +
					"`sections` array whose entries hold a `cells` array of `widgets` (`{\"widget\": \"<key>\"}`); valid widget keys are " +
					"listed by the Reco API endpoint GET /api/v1/external-api/dashboards/widgets. Reco rewrites SCIM filter " +
					"shorthand (`{\"scim\": \"...\"}`) into structured filters when saving, so use structured filters to avoid a " +
					"permanent diff.",
				Required:   true,
				Validators: []validator.String{validJSON{}},
			},
			"visibility": schema.StringAttribute{
				Description: "Who can see the dashboard: DASHBOARD_VISIBILITY_PRIVATE, DASHBOARD_VISIBILITY_SHARED_READ, " +
					"or DASHBOARD_VISIBILITY_SHARED_WRITE. Defaults to private.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.OneOf(dashboardVisibilities...)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"folder_path": schema.StringAttribute{
				Description: `Folder path of the dashboard, e.g. "/" or "/team/security". Defaults to "/".`,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_by": schema.StringAttribute{
				Description: "Email of the dashboard owner (read-only).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_by_name": schema.StringAttribute{
				Description: "Display name of the dashboard owner (read-only).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			attrCreatedAt: schema.StringAttribute{
				Description: "Timestamp when the dashboard was created (read-only).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Description: "Timestamp when the dashboard was last updated (read-only).",
				Computed:    true,
			},
		},
	}
}

func (r *customDashboardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &r.client, &resp.Diagnostics)
}

func (r *customDashboardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customDashboardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	saved, err := r.client.SaveDashboard(ctx, modelToSaveDashboard(&plan, ""))
	if err != nil {
		resp.Diagnostics.AddError("Error creating dashboard", err.Error())
		return
	}
	if saved.ID == "" {
		resp.Diagnostics.AddError("Error creating dashboard", "the Reco API did not return a dashboard ID")
		return
	}

	dashboardToModel(saved, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customDashboardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customDashboardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetDashboardByID(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading dashboard", err.Error())
		return
	}

	dashboardToModel(d, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *customDashboardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state customDashboardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	saved, err := r.client.SaveDashboard(ctx, modelToSaveDashboard(&plan, state.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error updating dashboard", err.Error())
		return
	}

	dashboardToModel(saved, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the dashboard: the external API has no delete endpoint.
func (*customDashboardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customDashboardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.AddWarning(
		"Dashboard not deleted in Reco",
		fmt.Sprintf("The Reco API does not support deleting dashboards. Dashboard %q (%s) was removed from Terraform state "+
			"but still exists in Reco; delete it in the Reco UI if it is no longer needed.",
			state.Name.ValueString(), state.ID.ValueString()),
	)
}

func (r *customDashboardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	d, err := r.client.GetDashboardByID(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing dashboard", fmt.Sprintf("cannot find dashboard %q: %s", req.ID, err))
		return
	}
	var state customDashboardResourceModel
	dashboardToModel(d, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func modelToSaveDashboard(m *customDashboardResourceModel, id string) *client.SaveDashboardRequest {
	return &client.SaveDashboardRequest{
		ID:         id,
		Name:       m.Name.ValueString(),
		Spec:       m.Spec.ValueString(),
		Visibility: m.Visibility.ValueString(),
		FolderPath: m.FolderPath.ValueString(),
	}
}

// dashboardToModel copies API-returned fields. The configured spec is kept
// when it is JSON-equivalent to the server's, so formatting differences
// produce no diff.
func dashboardToModel(d *client.Dashboard, m *customDashboardResourceModel) {
	m.ID = types.StringValue(d.ID)
	m.Name = types.StringValue(d.Name)
	if m.Spec.IsNull() || m.Spec.IsUnknown() || !jsonEquivalent(m.Spec.ValueString(), d.Spec) {
		m.Spec = types.StringValue(d.Spec)
	}
	m.Visibility = types.StringValue(d.Visibility)
	m.FolderPath = types.StringValue(d.FolderPath)
	m.CreatedBy = types.StringValue(d.CreatedBy)
	m.CreatedByName = types.StringValue(d.CreatedByName)
	m.CreatedAt = tfutil.StringPtrValue(d.CreatedAt)
	m.UpdatedAt = tfutil.StringPtrValue(d.UpdatedAt)
}

func jsonEquivalent(a, b string) bool {
	var av, bv any
	if json.Unmarshal([]byte(a), &av) != nil || json.Unmarshal([]byte(b), &bv) != nil {
		return a == b
	}
	ab, _ := json.Marshal(av)
	bb, _ := json.Marshal(bv)
	return bytes.Equal(ab, bb)
}

type validJSON struct{}

func (validJSON) Description(context.Context) string             { return "value must be valid JSON" }
func (v validJSON) MarkdownDescription(c context.Context) string { return v.Description(c) }

func (validJSON) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !json.Valid([]byte(req.ConfigValue.ValueString())) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON", "The value must be a valid JSON document.")
	}
}
