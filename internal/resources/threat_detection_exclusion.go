package resources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	tfutil "github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

var (
	_ resource.Resource                          = &threatDetectionExclusionResource{}
	_ resource.ResourceWithImportState           = &threatDetectionExclusionResource{}
	_ resource.ResourceWithConfigure             = &threatDetectionExclusionResource{}
	_ resource.ResourceWithValidateConfig        = &threatDetectionExclusionResource{}
	_ basetypes.StringTypable                    = rfc3339Type{}
	_ basetypes.StringValuableWithSemanticEquals = rfc3339Value{}
)

const (
	attrExpression  = "expression"
	attrIsGlobal    = "is_global"
	attrPolicyIDs   = "policy_ids"
	attrInstanceIDs = "instance_ids"
	attrState       = "state"
	attrValidUntil  = "valid_until"

	validUntilExample = "2026-12-31T00:00:00Z"
)

var exclusionStates = []string{
	client.ExclusionStateOnPermanent,
	client.ExclusionStateOnTemporary,
	client.ExclusionStateOff,
}

type threatDetectionExclusionResource struct {
	client *client.Client
}

func NewThreatDetectionExclusionResource() resource.Resource {
	return &threatDetectionExclusionResource{}
}

type threatDetectionExclusionResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Expression  types.String `tfsdk:"expression"`
	IsGlobal    types.Bool   `tfsdk:"is_global"`
	PolicyIDs   types.List   `tfsdk:"policy_ids"`
	InstanceIDs types.List   `tfsdk:"instance_ids"`
	State       types.String `tfsdk:"state"`
	ValidUntil  rfc3339Value `tfsdk:"valid_until"`
	PolicyID    types.String `tfsdk:"policy_id"`
	PolicyName  types.String `tfsdk:"policy_name"`
	Apps        types.List   `tfsdk:"apps"`
	CreatedBy   types.String `tfsdk:"created_by"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func (r *threatDetectionExclusionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_threat_detection_exclusion"
}

func (r *threatDetectionExclusionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = threatDetectionExclusionSchema()
}

func threatDetectionExclusionSchema() schema.Schema {
	return schema.Schema{
		Description: "Manages a Reco threat detection exclusion that suppresses matching alerts. " +
			"State changes (on, on until valid_until, or off) use the exclusion state API. " +
			"Destroy deletes the exclusion.",
		Attributes: threatDetectionExclusionAttributes(),
	}
}

func threatDetectionExclusionAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "Exclusion ID (stable UUID, set after create).",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		attrName: schema.StringAttribute{
			Description: "Display name of the exclusion.",
			Required:    true,
		},
		attrDescription: schema.StringAttribute{
			Description: "Longer description shown in the Reco UI. Omit to leave it empty.",
			Optional:    true,
		},
		attrExpression: schema.StringAttribute{
			Description: "JSONata expression evaluated against each policy violation. A violation is suppressed when it evaluates to true.",
			Required:    true,
		},
		attrIsGlobal: schema.BoolAttribute{
			Description: "Apply the exclusion to every threat detection policy. policy_ids and instance_ids must then be omitted.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		attrPolicyIDs: schema.ListAttribute{
			Description: "Threat detection policy IDs this exclusion applies to. Required unless is_global is true.",
			Optional:    true,
			ElementType: types.StringType,
		},
		attrInstanceIDs: schema.ListAttribute{
			Description: "Restrict the exclusion to violations from these integration instances. Omit to include every instance.",
			Optional:    true,
			ElementType: types.StringType,
		},
		attrState: schema.StringAttribute{
			Description: "VIOLATION_EXCLUSION_STATE_ON_PERMANENT (default), VIOLATION_EXCLUSION_STATE_ON_TEMPORARY, " +
				"or VIOLATION_EXCLUSION_STATE_OFF. Changing it calls the exclusion state API.",
			Optional:   true,
			Computed:   true,
			Default:    stringdefault.StaticString(client.ExclusionStateOnPermanent),
			Validators: []validator.String{stringvalidator.OneOf(exclusionStates...)},
		},
		attrValidUntil: schema.StringAttribute{
			Description: "RFC3339 timestamp when an ON_TEMPORARY exclusion turns off, for example " + validUntilExample +
				". Required when state is VIOLATION_EXCLUSION_STATE_ON_TEMPORARY. Omit it otherwise.",
			CustomType: rfc3339Type{},
			Optional:   true,
		},
		"policy_id": schema.StringAttribute{
			Description: "Policy ID when the exclusion targets exactly one policy (read-only).",
			Computed:    true,
		},
		"policy_name": schema.StringAttribute{
			Description: "Name of policy_id (read-only).",
			Computed:    true,
		},
		"apps": schema.ListAttribute{
			Description: "Applications the exclusion applies to (read-only).",
			Computed:    true,
			ElementType: types.StringType,
		},
		"created_by": schema.StringAttribute{
			Description: "User who created the exclusion (read-only).",
			Computed:    true,
		},
		attrCreatedAt: schema.StringAttribute{
			Description: "Creation timestamp (read-only).",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"updated_at": schema.StringAttribute{
			Description: "Last update timestamp (read-only).",
			Computed:    true,
		},
	}
}

func (r *threatDetectionExclusionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &r.client, &resp.Diagnostics)
}

func (r *threatDetectionExclusionResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg threatDetectionExclusionResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateExclusionScope(&cfg, &resp.Diagnostics)
	validateExclusionWindow(&cfg, &resp.Diagnostics)
}

func (r *threatDetectionExclusionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan threatDetectionExclusionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := r.client.CreateThreatDetectionExclusion(ctx, &client.CreateThreatDetectionExclusionRequest{
		Definition: exclusionDefinition(&plan),
		State:      plan.State.ValueString(),
		ValidUntil: exclusionValidUntil(&plan),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating threat detection exclusion", err.Error())
		return
	}
	plan.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), plan.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	full, err := r.client.GetThreatDetectionExclusionByID(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading created threat detection exclusion",
			fmt.Sprintf("exclusion %s was created but could not be read: %s", id, err),
		)
		return
	}
	exclusionToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *threatDetectionExclusionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state threatDetectionExclusionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	exclusion, err := r.client.GetThreatDetectionExclusionByID(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading threat detection exclusion", err.Error())
		return
	}

	exclusionToModel(exclusion, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *threatDetectionExclusionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan threatDetectionExclusionResourceModel
	var state threatDetectionExclusionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID

	if exclusionDefinitionChanged(&plan, &state) {
		if err := r.client.UpdateThreatDetectionExclusion(ctx, &client.UpdateThreatDetectionExclusionRequest{
			ID:         state.ID.ValueString(),
			Definition: exclusionDefinition(&plan),
		}); err != nil {
			resp.Diagnostics.AddError("Error updating threat detection exclusion", err.Error())
			return
		}
	}
	if exclusionLifecycleChanged(&plan, &state) {
		if err := r.client.SetThreatDetectionExclusionState(ctx, client.SetThreatDetectionExclusionStateRequest{
			ID:         state.ID.ValueString(),
			State:      plan.State.ValueString(),
			ValidUntil: exclusionValidUntil(&plan),
		}); err != nil {
			resp.Diagnostics.AddError("Error updating threat detection exclusion state", err.Error())
			return
		}
	}

	full, err := r.client.GetThreatDetectionExclusionByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated threat detection exclusion", err.Error())
		return
	}
	exclusionToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *threatDetectionExclusionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state threatDetectionExclusionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteThreatDetectionExclusion(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting threat detection exclusion", err.Error())
	}
}

func (r *threatDetectionExclusionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	exclusion, err := r.client.GetThreatDetectionExclusionByID(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing threat detection exclusion", fmt.Sprintf("cannot find exclusion %q: %s", req.ID, err))
		return
	}
	var state threatDetectionExclusionResourceModel
	exclusionToModel(exclusion, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func validateExclusionScope(cfg *threatDetectionExclusionResourceModel, diags *diag.Diagnostics) {
	if cfg.IsGlobal.IsUnknown() {
		return
	}
	global := !cfg.IsGlobal.IsNull() && cfg.IsGlobal.ValueBool()
	policyCount, policiesKnown := knownListLen(cfg.PolicyIDs)
	instanceCount, instancesKnown := knownListLen(cfg.InstanceIDs)
	if global && policiesKnown && policyCount > 0 {
		diags.AddAttributeError(path.Root(attrPolicyIDs), "Invalid exclusion scope",
			"policy_ids must be omitted when is_global is true.")
	}
	if global && instancesKnown && instanceCount > 0 {
		diags.AddAttributeError(path.Root(attrInstanceIDs), "Invalid exclusion scope",
			"instance_ids must be omitted when is_global is true.")
	}
	if !global && policiesKnown && policyCount == 0 {
		diags.AddAttributeError(path.Root(attrPolicyIDs), "Invalid exclusion scope",
			"Set policy_ids, or set is_global to true to suppress alerts on every threat detection policy.")
	}
}

func validateExclusionWindow(cfg *threatDetectionExclusionResourceModel, diags *diag.Diagnostics) {
	if cfg.State.IsUnknown() || cfg.ValidUntil.IsUnknown() {
		return
	}
	state := client.ExclusionStateOnPermanent
	if !cfg.State.IsNull() && cfg.State.ValueString() != "" {
		state = cfg.State.ValueString()
	}
	raw := ""
	if !cfg.ValidUntil.IsNull() {
		raw = cfg.ValidUntil.ValueString()
	}
	if raw != "" {
		if _, err := time.Parse(time.RFC3339, raw); err != nil {
			diags.AddAttributeError(path.Root(attrValidUntil), "Invalid valid_until",
				fmt.Sprintf("valid_until %q is not a valid RFC3339 timestamp. Example: %s.", raw, validUntilExample))
		}
	}
	if state == client.ExclusionStateOnTemporary && raw == "" {
		diags.AddAttributeError(path.Root(attrValidUntil), "Missing valid_until",
			"valid_until is required when state is VIOLATION_EXCLUSION_STATE_ON_TEMPORARY (ON_TEMPORARY). "+
				"Set an RFC3339 timestamp, for example "+validUntilExample+".")
	}
	if state != client.ExclusionStateOnTemporary && raw != "" {
		diags.AddAttributeError(path.Root(attrValidUntil), "Unexpected valid_until",
			"Omit valid_until unless state is VIOLATION_EXCLUSION_STATE_ON_TEMPORARY. "+
				"Permanent and OFF exclusions do not expire.")
	}
}

func knownListLen(list types.List) (int, bool) {
	if list.IsUnknown() {
		return 0, false
	}
	if list.IsNull() {
		return 0, true
	}
	return len(list.Elements()), true
}

func exclusionDefinition(m *threatDetectionExclusionResourceModel) client.ThreatDetectionExclusionDefinition {
	return client.ThreatDetectionExclusionDefinition{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueString(),
		Expression:  m.Expression.ValueString(),
		IsGlobal:    m.IsGlobal.ValueBool(),
		PolicyIDs:   tfutil.ListToStringSlice(m.PolicyIDs),
		InstanceIDs: tfutil.ListToStringSlice(m.InstanceIDs),
	}
}

func exclusionValidUntil(m *threatDetectionExclusionResourceModel) *string {
	if m.State.ValueString() != client.ExclusionStateOnTemporary || m.ValidUntil.IsNull() || m.ValidUntil.ValueString() == "" {
		return nil
	}
	value := m.ValidUntil.ValueString()
	return &value
}

func exclusionDefinitionChanged(plan, state *threatDetectionExclusionResourceModel) bool {
	return !plan.Name.Equal(state.Name) ||
		!plan.Description.Equal(state.Description) ||
		!plan.Expression.Equal(state.Expression) ||
		!plan.IsGlobal.Equal(state.IsGlobal) ||
		!plan.PolicyIDs.Equal(state.PolicyIDs) ||
		!plan.InstanceIDs.Equal(state.InstanceIDs)
}

func exclusionLifecycleChanged(plan, state *threatDetectionExclusionResourceModel) bool {
	if !plan.State.Equal(state.State) {
		return true
	}
	return !timestampsEqual(plan.ValidUntil, state.ValidUntil)
}

func exclusionToModel(exclusion *client.ThreatDetectionExclusion, model *threatDetectionExclusionResourceModel) {
	model.ID = types.StringValue(exclusion.ID)
	model.Name = types.StringValue(exclusion.Name)
	model.Description = syncOptionalString(model.Description, exclusion.Description)
	model.Expression = types.StringValue(exclusion.Expression)
	model.IsGlobal = types.BoolValue(exclusion.IsGlobal)
	model.PolicyIDs = syncOptionalList(model.PolicyIDs, exclusion.PolicyIDs)
	model.InstanceIDs = syncOptionalList(model.InstanceIDs, exclusion.InstanceIDs)
	model.State = types.StringValue(exclusion.State)
	model.ValidUntil = syncOptionalTimestamp(derefString(exclusion.ValidUntil))
	model.PolicyID = syncOptionalString(model.PolicyID, exclusion.PolicyID)
	model.PolicyName = syncOptionalString(model.PolicyName, exclusion.PolicyName)
	model.Apps = tfutil.StringSliceToList(exclusion.Apps)
	model.CreatedBy = syncOptionalString(model.CreatedBy, exclusion.CreatedBy)
	model.CreatedAt = tfutil.StringPtrValue(exclusion.CreatedAt)
	model.UpdatedAt = tfutil.StringPtrValue(exclusion.UpdatedAt)
}

func syncOptionalString(prior types.String, apiValue string) types.String {
	if prior.IsUnknown() {
		prior = types.StringNull()
	}
	if apiValue == "" {
		if prior.IsNull() || prior.ValueString() == "" {
			return prior
		}
		return types.StringNull()
	}
	return types.StringValue(apiValue)
}

func syncOptionalList(prior types.List, apiValues []string) types.List {
	if prior.IsUnknown() || prior.ElementType(context.Background()) == nil {
		prior = types.ListNull(types.StringType)
	}
	if len(apiValues) == 0 {
		if prior.IsNull() || len(prior.Elements()) == 0 {
			return prior
		}
		return types.ListNull(types.StringType)
	}
	return tfutil.StringSliceToList(apiValues)
}

func syncOptionalTimestamp(apiValue string) rfc3339Value {
	if strings.TrimSpace(apiValue) == "" {
		return rfc3339Null()
	}
	return rfc3339Value{StringValue: basetypes.NewStringValue(apiValue)}
}

func timestampsEqual(left, right rfc3339Value) bool {
	if left.Equal(right) {
		return true
	}
	equal, diags := left.StringSemanticEquals(context.Background(), right)
	return !diags.HasError() && equal
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type rfc3339Type struct {
	basetypes.StringType
}

func (t rfc3339Type) Equal(o attr.Type) bool {
	_, ok := o.(rfc3339Type)
	return ok
}

func (t rfc3339Type) String() string { return "rfc3339Type" }

func (t rfc3339Type) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return rfc3339Value{StringValue: in}, nil
}

func (t rfc3339Type) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	value, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	stringValue, ok := value.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", value)
	}
	return rfc3339Value{StringValue: stringValue}, nil
}

func (t rfc3339Type) ValueType(context.Context) attr.Value {
	return rfc3339Value{}
}

type rfc3339Value struct {
	basetypes.StringValue
}

func rfc3339Null() rfc3339Value {
	return rfc3339Value{StringValue: basetypes.NewStringNull()}
}

func (v rfc3339Value) Type(context.Context) attr.Type { return rfc3339Type{} }

func (v rfc3339Value) Equal(other attr.Value) bool {
	o, ok := other.(rfc3339Value)
	if !ok {
		return false
	}
	return v.StringValue.Equal(o.StringValue)
}

func (v rfc3339Value) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	other, ok := newValuable.(rfc3339Value)
	if !ok {
		return false, diag.Diagnostics{diag.NewErrorDiagnostic("Invalid valid_until comparison", fmt.Sprintf("expected rfc3339Value, got %T", newValuable))}
	}
	if v.IsNull() || other.IsNull() || v.IsUnknown() || other.IsUnknown() {
		return false, nil
	}
	left, leftErr := time.Parse(time.RFC3339, v.ValueString())
	right, rightErr := time.Parse(time.RFC3339, other.ValueString())
	if leftErr != nil || rightErr != nil {
		return false, nil
	}
	return left.Equal(right), nil
}
