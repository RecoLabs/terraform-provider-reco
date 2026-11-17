package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	tfutil "github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

var (
	_ resource.Resource                   = &complianceFrameworkResource{}
	_ resource.ResourceWithImportState    = &complianceFrameworkResource{}
	_ resource.ResourceWithConfigure      = &complianceFrameworkResource{}
	_ resource.ResourceWithValidateConfig = &complianceFrameworkResource{}
)

const (
	attrControls        = "controls"
	attrControlID       = "control_id"
	attrParentControlID = "parent_control_id"
	attrRelatedPolicies = "related_policy_ids"
	attrManual          = "manual"
	attrSkipped         = "skipped"

	// Limits enforced by the External API.
	maxFrameworkTitleLen   = 256
	maxFrameworkTextLen    = 4096
	maxFrameworkTagLen     = 64
	maxControlIDLen        = 256
	maxControls            = 1000
	maxRelatedPoliciesEach = 200
)

type complianceFrameworkResource struct {
	client *client.Client
}

func NewComplianceFrameworkResource() resource.Resource {
	return &complianceFrameworkResource{}
}

type complianceFrameworkResourceModel struct {
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

type complianceControlModel struct {
	ControlID        types.String `tfsdk:"control_id"`
	Title            types.String `tfsdk:"title"`
	Description      types.String `tfsdk:"description"`
	ParentControlID  types.String `tfsdk:"parent_control_id"`
	RelatedPolicyIDs types.Set    `tfsdk:"related_policy_ids"`
	Manual           types.Bool   `tfsdk:"manual"`
	Skipped          types.Bool   `tfsdk:"skipped"`
}

func (r *complianceFrameworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_compliance_framework"
}

func (r *complianceFrameworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a custom Reco compliance framework with its controls, for example to promote the same " +
			"framework from a development tenant to a production tenant. Built-in frameworks cannot be managed.\n\n" +
			"The Reco API replaces a framework's controls as a whole on every update, so this resource owns the " +
			"framework's controls: `controls` is the full list, and an update without `controls` removes every control " +
			"(including controls added in the Reco UI). The API cannot return a framework's controls, so changes made " +
			"to them outside Terraform are not detected.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Compliance framework ID (UUID, set after create).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			attrTitle: schema.StringAttribute{
				Description: "Framework display name.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.UTF8LengthBetween(1, maxFrameworkTitleLen)},
			},
			attrDescription: schema.StringAttribute{
				Description: "Longer description shown in the Reco UI.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{stringvalidator.UTF8LengthAtMost(maxFrameworkTextLen)},
			},
			"version": schema.StringAttribute{
				Description: "Framework version string, e.g. `2024.1`.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{stringvalidator.UTF8LengthAtMost(maxFrameworkTagLen)},
			},
			"type": schema.StringAttribute{
				Description: "Free-text framework type.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{stringvalidator.UTF8LengthAtMost(maxFrameworkTagLen)},
			},
			"is_active": schema.BoolAttribute{
				Description: "Whether the framework is active. Defaults to `false`. The Reco API only applies this when the " +
					"framework is created, so changing it forces the framework to be destroyed and recreated, which also " +
					"deletes its controls and changes its ID. To activate or deactivate a framework without recreating it, " +
					"leave this unset and use the Reco UI; the current state is read back.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
					boolplanmodifier.RequiresReplace(),
				},
			},
			attrControls: complianceControlsAttribute(),
			attrCreatedAt: schema.StringAttribute{
				Description: "When the framework was created (read-only).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Description: "When the framework was last updated (read-only).",
				Computed:    true,
			},
		},
	}
}

func complianceControlsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "The framework's complete list of controls (at most 1000). Omitting it removes every control from " +
			"the framework. " + writeOnlyNote + " After import, add the controls to the configuration before applying " +
			"any change, because applying replaces the framework's controls with the configured ones.",
		Optional: true,
		Validators: []validator.List{
			listvalidator.SizeAtMost(maxControls),
		},
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				attrControlID: schema.StringAttribute{
					Description: "Control identifier within the framework, e.g. `CC6.1`. Must be unique in the framework.",
					Required:    true,
					Validators:  []validator.String{stringvalidator.UTF8LengthBetween(1, maxControlIDLen)},
				},
				attrTitle: schema.StringAttribute{
					Description: "Control display name.",
					Optional:    true,
					Validators:  []validator.String{stringvalidator.UTF8LengthAtMost(maxControlIDLen)},
				},
				attrDescription: schema.StringAttribute{
					Description: "Control description.",
					Optional:    true,
					Validators:  []validator.String{stringvalidator.UTF8LengthAtMost(maxFrameworkTextLen)},
				},
				attrParentControlID: schema.StringAttribute{
					Description: "`control_id` of the parent control, when this control is nested under another.",
					Optional:    true,
					Validators:  []validator.String{stringvalidator.UTF8LengthAtMost(maxControlIDLen)},
				},
				attrRelatedPolicies: schema.SetAttribute{
					Description: "IDs of the posture checks mapped to this control (for example `reco_posture_check.example.id`). " +
						"Each posture check must exist.",
					Optional:    true,
					ElementType: types.StringType,
					Validators: []validator.Set{
						setvalidator.SizeAtMost(maxRelatedPoliciesEach),
						setvalidator.ValueStringsAre(stringvalidator.RegexMatches(tfutil.UUIDPattern, "must be a UUID")),
					},
				},
				attrManual: schema.BoolAttribute{
					Description: "Whether the control is evaluated manually. Defaults to `false`.",
					Optional:    true,
				},
				attrSkipped: schema.BoolAttribute{
					Description: "Whether the control is skipped. Defaults to `false`.",
					Optional:    true,
				},
			},
		},
	}
}

// ValidateConfig rejects what the API rejects in a framework's controls: duplicate control IDs, and parents that are
// unknown, the control itself or part of a cycle. The API checks them only in the upsert, which Create runs after
// the framework exists, so a bad config would otherwise leave a tainted framework behind.
func (r *complianceFrameworkResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var controls types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(attrControls), &controls)...)
	if resp.Diagnostics.HasError() || controls.IsNull() || controls.IsUnknown() {
		return
	}
	var models []complianceControlModel
	resp.Diagnostics.Append(controls.ElementsAs(ctx, &models, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateControlIDs(models, &resp.Diagnostics)
	validateControlParents(models, &resp.Diagnostics)
}

func validateControlIDs(models []complianceControlModel, diags *diag.Diagnostics) {
	seen := make(map[string]int, len(models))
	for i := range models {
		controlID := models[i].ControlID
		if controlID.IsNull() || controlID.IsUnknown() {
			continue
		}
		id := controlID.ValueString()
		if first, dup := seen[id]; dup {
			diags.AddAttributeError(
				path.Root(attrControls).AtListIndex(i).AtName(attrControlID),
				"Duplicate control ID",
				fmt.Sprintf("control_id %q is already used by controls[%d]; control IDs must be unique in a framework.", id, first),
			)
			continue
		}
		seen[id] = i
	}
}

// validateControlParents rejects the parent trees the API rejects: a parent that is not a control of the
// framework, a control that is its own parent, and parent cycles. Without this the error would come from the
// upsert in Create, after the framework already exists. Nothing is checked while a control_id is unknown,
// since a missing parent might be that control.
func validateControlParents(models []complianceControlModel, diags *diag.Diagnostics) {
	parents := make(map[string]string, len(models))
	for i := range models {
		controlID := models[i].ControlID
		if controlID.IsUnknown() || models[i].ParentControlID.IsUnknown() {
			return
		}
		if !controlID.IsNull() {
			parents[controlID.ValueString()] = models[i].ParentControlID.ValueString()
		}
	}
	for i := range models {
		id, parent := models[i].ControlID.ValueString(), models[i].ParentControlID.ValueString()
		parentPath := path.Root(attrControls).AtListIndex(i).AtName(attrParentControlID)
		switch {
		case parent == "":
		case parent == id:
			diags.AddAttributeError(parentPath, "Control is its own parent", fmt.Sprintf("control %q cannot be its own parent_control_id.", id))
		case !hasControl(parents, parent):
			diags.AddAttributeError(parentPath, "Unknown parent control",
				fmt.Sprintf("parent_control_id %q is not the control_id of any control in this framework.", parent))
		case inParentCycle(id, parents):
			diags.AddAttributeError(parentPath, "Parent control cycle", fmt.Sprintf("control %q is part of a parent_control_id cycle.", id))
		}
	}
}

func hasControl(parents map[string]string, id string) bool {
	_, ok := parents[id]
	return ok
}

// inParentCycle reports whether following parents from start leads back to start. A control that only leads
// into a cycle is not in it.
func inParentCycle(start string, parents map[string]string) bool {
	seen := map[string]bool{}
	for current := parents[start]; current != "" && !seen[current]; current = parents[current] {
		if current == start {
			return true
		}
		seen[current] = true
	}
	return false
}

func (r *complianceFrameworkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &r.client, &resp.Diagnostics)
}

func (r *complianceFrameworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan complianceFrameworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	controls, diags := complianceControlsFromList(ctx, plan.Controls)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := r.client.CreateComplianceFramework(ctx, &client.ComplianceFrameworkDefinition{
		Title:       plan.Title.ValueString(),
		Description: plan.Description.ValueString(),
		Version:     plan.Version.ValueString(),
		Type:        plan.Type.ValueString(),
		IsActive:    plan.IsActive.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating compliance framework", err.Error())
		return
	}
	// Keep the ID in state from here on, so a failure below leaves a tainted
	// resource that is replaced on the next apply instead of an orphan.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	plan.ID = types.StringValue(id)

	if err := r.readInto(ctx, id, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading created compliance framework", err.Error())
		return
	}

	// Create takes metadata only; the controls are added by an upsert.
	if len(controls) > 0 {
		if err := r.client.UpsertComplianceFramework(ctx, upsertRequest(&plan, controls)); err != nil {
			resp.Diagnostics.AddError("Error adding controls to created compliance framework", err.Error())
			// The server has no controls, so state must not claim it does: after an untaint a plan would show
			// no diff and the framework would stay empty.
			plan.Controls = types.ListNull(plan.Controls.ElementType(ctx))
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}
		if err := r.readInto(ctx, id, &plan); err != nil {
			resp.Diagnostics.AddError("Error reading created compliance framework", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *complianceFrameworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state complianceFrameworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Controls cannot be read from the API, so state keeps the last applied list.
	if err := r.readInto(ctx, state.ID.ValueString(), &state); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading compliance framework", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *complianceFrameworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state complianceFrameworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID

	controls, diags := complianceControlsFromList(ctx, plan.Controls)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The upsert always carries the configured controls: leaving them out would delete them.
	if err := r.client.UpsertComplianceFramework(ctx, upsertRequest(&plan, controls)); err != nil {
		resp.Diagnostics.AddError("Error updating compliance framework", err.Error())
		return
	}
	if err := r.readInto(ctx, state.ID.ValueString(), &plan); err != nil {
		resp.Diagnostics.AddError("Error reading updated compliance framework", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *complianceFrameworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state complianceFrameworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteComplianceFramework(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting compliance framework", err.Error())
	}
}

func (r *complianceFrameworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	framework, err := r.client.GetComplianceFrameworkByID(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing compliance framework", fmt.Sprintf("cannot find compliance framework %q: %s", req.ID, err))
		return
	}
	if !framework.IsCustom {
		resp.Diagnostics.AddError(
			"Cannot import a built-in compliance framework",
			fmt.Sprintf("%q (%s) is a built-in framework, which the Reco API does not allow editing or deleting. "+
				"Only custom frameworks can be managed.", framework.Title, req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), framework.ID)...)
}

func (r *complianceFrameworkResource) readInto(ctx context.Context, id string, m *complianceFrameworkResourceModel) error {
	framework, err := r.client.GetComplianceFrameworkByID(ctx, id)
	if err != nil {
		return err
	}
	complianceFrameworkToModel(framework, m)
	return nil
}

func complianceFrameworkToModel(f *client.ComplianceFramework, m *complianceFrameworkResourceModel) {
	m.ID = types.StringValue(f.ID)
	m.Title = types.StringValue(f.Title)
	m.Description = types.StringValue(f.Description)
	m.Version = types.StringValue(f.Version)
	m.Type = types.StringValue(f.Type)
	m.IsActive = types.BoolValue(f.IsActive)
	m.CreatedAt = types.StringValue(f.CreatedAt)
	m.UpdatedAt = types.StringValue(f.UpdatedAt)
}

func upsertRequest(m *complianceFrameworkResourceModel, controls []client.ComplianceControl) *client.UpsertComplianceFrameworkRequest {
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

func complianceControlsFromList(ctx context.Context, l types.List) ([]client.ComplianceControl, diag.Diagnostics) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	var models []complianceControlModel
	diags := l.ElementsAs(ctx, &models, false)
	if diags.HasError() {
		return nil, diags
	}
	controls := make([]client.ComplianceControl, 0, len(models))
	for i := range models {
		m := &models[i]
		control := client.ComplianceControl{
			ControlID:       m.ControlID.ValueString(),
			Title:           m.Title.ValueString(),
			Description:     m.Description.ValueString(),
			ParentControlID: m.ParentControlID.ValueString(),
			Manual:          m.Manual.ValueBool(),
			Skipped:         m.Skipped.ValueBool(),
		}
		control.RelatedPolicyIDs = tfutil.SetToStringSlice(m.RelatedPolicyIDs)
		controls = append(controls, control)
	}
	return controls, diags
}
