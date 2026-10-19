package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	tfutil "github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

var (
	_ resource.Resource                = &customComplianceFrameworkResource{}
	_ resource.ResourceWithImportState = &customComplianceFrameworkResource{}
	_ resource.ResourceWithConfigure   = &customComplianceFrameworkResource{}
)

const (
	attrControlID        = "control_id"
	attrParentControlID  = "parent_control_id"
	attrRelatedPolicyIDs = "related_policy_ids"
	attrManual           = "manual"
	attrSkipped          = "skipped"
)

var complianceControlAttrTypes = map[string]attr.Type{
	attrControlID:        types.StringType,
	attrTitle:            types.StringType,
	attrDescription:      types.StringType,
	attrParentControlID:  types.StringType,
	attrRelatedPolicyIDs: types.ListType{ElemType: types.StringType},
	attrManual:           types.BoolType,
	attrSkipped:          types.BoolType,
}

type customComplianceFrameworkResource struct {
	client *client.Client
}

func NewCustomComplianceFrameworkResource() resource.Resource {
	return &customComplianceFrameworkResource{}
}

type customComplianceFrameworkResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Title       types.String `tfsdk:"title"`
	Description types.String `tfsdk:"description"`
	Version     types.String `tfsdk:"version"`
	Type        types.String `tfsdk:"type"`
	IsActive    types.Bool   `tfsdk:"is_active"`
	Controls    types.List   `tfsdk:"controls"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func (r *customComplianceFrameworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_compliance_framework"
}

func (r *customComplianceFrameworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Reco custom compliance framework and its controls. Each control maps to one or more " +
			"posture checks. Built-in frameworks cannot be managed with this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Framework ID (set after create).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			attrTitle: schema.StringAttribute{
				Description: "Framework display name.",
				Required:    true,
			},
			attrDescription: schema.StringAttribute{
				Description: "Longer description shown in the Reco UI.",
				Optional:    true,
				Computed:    true,
			},
			"version": schema.StringAttribute{
				Description: "Framework version string.",
				Optional:    true,
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "Free-text framework type.",
				Optional:    true,
				Computed:    true,
			},
			"is_active": schema.BoolAttribute{
				Description: "Whether the framework is active. The Reco API applies this value only when the framework " +
					"is created, so changing it forces the framework to be destroyed and recreated. " +
					"When omitted, the framework is created inactive and the value reported by Reco is tracked.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
					boolplanmodifier.RequiresReplace(),
				},
			},
			"controls": schema.ListNestedAttribute{
				Description: "Controls of the framework. Every update replaces the full control list, so omitting this " +
					"attribute removes all existing controls. " + writeOnlyNote,
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						attrControlID: schema.StringAttribute{
							Description: "Control identifier within the framework.",
							Required:    true,
						},
						attrTitle: schema.StringAttribute{
							Description: "Control display name.",
							Required:    true,
						},
						attrDescription: schema.StringAttribute{
							Description: "Control description.",
							Optional:    true,
						},
						attrParentControlID: schema.StringAttribute{
							Description: "The control_id of the parent control, when this control is nested.",
							Optional:    true,
						},
						attrRelatedPolicyIDs: schema.ListAttribute{
							Description: "IDs (UUIDs) of the posture checks mapped to this control.",
							Optional:    true,
							ElementType: types.StringType,
						},
						attrManual: schema.BoolAttribute{
							Description: "Whether the control is evaluated manually.",
							Optional:    true,
						},
						attrSkipped: schema.BoolAttribute{
							Description: "Whether the control is skipped.",
							Optional:    true,
						},
					},
				},
			},
			attrCreatedAt: schema.StringAttribute{
				Description: "Timestamp when the framework was created (read-only).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Description: "Timestamp when the framework was last updated (read-only).",
				Computed:    true,
			},
		},
	}
}

func (r *customComplianceFrameworkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &r.client, &resp.Diagnostics)
}

func (r *customComplianceFrameworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customComplianceFrameworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	def := client.ComplianceFrameworkDefinition{
		Title:       plan.Title.ValueString(),
		Description: plan.Description.ValueString(),
		Version:     plan.Version.ValueString(),
		Type:        plan.Type.ValueString(),
		IsActive:    plan.IsActive.ValueBool(),
	}
	id, err := r.client.CreateComplianceFramework(ctx, &def)
	if err != nil {
		resp.Diagnostics.AddError("Error creating compliance framework", err.Error())
		return
	}
	plan.ID = types.StringValue(id)

	controls := modelToComplianceControls(&plan)
	if len(controls) > 0 {
		upsert := modelToComplianceUpsert(&plan, controls)
		if err := r.client.UpsertComplianceFramework(ctx, upsert); err != nil {
			// The framework exists without its controls; remove it so a retry starts clean.
			if delErr := r.client.DeleteComplianceFramework(ctx, id); delErr != nil && !client.IsNotFound(delErr) {
				resp.Diagnostics.AddError("Error cleaning up compliance framework",
					fmt.Sprintf("framework %q was created but adding controls failed and it could not be deleted: %s", id, delErr))
			}
			resp.Diagnostics.AddError("Error adding controls to compliance framework", err.Error())
			return
		}
	}

	full, err := r.client.GetComplianceFrameworkByID(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created compliance framework", err.Error())
		return
	}
	complianceFrameworkToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customComplianceFrameworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customComplianceFrameworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	fw, err := r.client.GetComplianceFrameworkByID(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading compliance framework", err.Error())
		return
	}

	complianceFrameworkToModel(fw, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *customComplianceFrameworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state customComplianceFrameworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID

	upsert := modelToComplianceUpsert(&plan, modelToComplianceControls(&plan))
	if err := r.client.UpsertComplianceFramework(ctx, upsert); err != nil {
		resp.Diagnostics.AddError("Error updating compliance framework", err.Error())
		return
	}

	full, err := r.client.GetComplianceFrameworkByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated compliance framework", err.Error())
		return
	}
	complianceFrameworkToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customComplianceFrameworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customComplianceFrameworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteComplianceFramework(ctx, state.ID.ValueString()); err != nil {
		if !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Error deleting compliance framework", err.Error())
		}
	}
}

func (r *customComplianceFrameworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fw, err := r.client.GetComplianceFrameworkByID(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing compliance framework", fmt.Sprintf("cannot find compliance framework %q: %s", req.ID, err))
		return
	}
	if !fw.IsCustom {
		resp.Diagnostics.AddError("Error importing compliance framework",
			fmt.Sprintf("compliance framework %q (%s) is built in; only custom frameworks can be managed", req.ID, fw.Title))
		return
	}

	state := customComplianceFrameworkResourceModel{
		Controls: types.ListNull(types.ObjectType{AttrTypes: complianceControlAttrTypes}),
	}
	complianceFrameworkToModel(fw, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func modelToComplianceUpsert(m *customComplianceFrameworkResourceModel, controls []client.ComplianceControl) *client.UpsertComplianceFrameworkRequest {
	return &client.UpsertComplianceFrameworkRequest{
		Framework: client.ComplianceFrameworkInput{
			ID:          m.ID.ValueString(),
			Title:       m.Title.ValueString(),
			Description: m.Description.ValueString(),
			Version:     m.Version.ValueString(),
			Type:        m.Type.ValueString(),
			IsActive:    m.IsActive.ValueBool(),
		},
		Controls: controls,
	}
}

func modelToComplianceControls(m *customComplianceFrameworkResourceModel) []client.ComplianceControl {
	if m.Controls.IsNull() || m.Controls.IsUnknown() {
		return nil
	}
	controls := make([]client.ComplianceControl, 0, len(m.Controls.Elements()))
	for _, elem := range m.Controls.Elements() {
		obj, ok := elem.(types.Object)
		if !ok {
			continue
		}
		attrs := obj.Attributes()
		c := client.ComplianceControl{
			ControlID: attrs[attrControlID].(types.String).ValueString(),
			Title:     attrs[attrTitle].(types.String).ValueString(),
		}
		c.Description = attrs[attrDescription].(types.String).ValueString()
		c.ParentControlID = attrs[attrParentControlID].(types.String).ValueString()
		c.RelatedPolicyIDs = tfutil.ListToStringSlice(attrs[attrRelatedPolicyIDs].(types.List))
		c.Manual = attrs[attrManual].(types.Bool).ValueBool()
		c.Skipped = attrs[attrSkipped].(types.Bool).ValueBool()
		controls = append(controls, c)
	}
	return controls
}

// complianceFrameworkToModel copies API-returned fields. Controls are never
// returned by the API, so the model's existing controls are left untouched.
func complianceFrameworkToModel(fw *client.ComplianceFramework, m *customComplianceFrameworkResourceModel) {
	m.ID = types.StringValue(fw.ID)
	m.Title = types.StringValue(fw.Title)
	m.Description = types.StringValue(fw.Description)
	m.Version = types.StringValue(fw.Version)
	m.Type = types.StringValue(fw.Type)
	m.IsActive = types.BoolValue(fw.IsActive)
	m.CreatedAt = tfutil.StringPtrValue(fw.CreatedAt)
	m.UpdatedAt = tfutil.StringPtrValue(fw.UpdatedAt)
}
