package resources

import (
	"context"
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
	_ resource.Resource                = &threatDetectionPolicyResource{}
	_ resource.ResourceWithImportState = &threatDetectionPolicyResource{}
	_ resource.ResourceWithConfigure   = &threatDetectionPolicyResource{}
)

type threatDetectionPolicyResource struct {
	client *client.Client
}

func NewThreatDetectionPolicyResource() resource.Resource {
	return &threatDetectionPolicyResource{}
}

type threatDetectionPolicyResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	Title               types.String `tfsdk:"title"`
	Description         types.String `tfsdk:"description"`
	DataSource          types.String `tfsdk:"data_source"`
	ViolationRiskLevel  types.String `tfsdk:"violation_risk_level"`
	ViolationRiskType   types.String `tfsdk:"violation_risk_type"`
	DefaultStatus       types.String `tfsdk:"default_status"`
	ConditionsJsonata   types.String `tfsdk:"conditions_jsonata"`
	DescriptionTemplate types.String `tfsdk:"description_template"`
	HowToRemediate      types.String `tfsdk:"how_to_remediate"`
	WhyShouldICare      types.String `tfsdk:"why_should_i_care"`
	Tags                types.List   `tfsdk:"tags"`
	Severity            types.String `tfsdk:"severity"`
	Status              types.String `tfsdk:"status"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

func (r *threatDetectionPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_threat_detection_policy"
}

func (r *threatDetectionPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Reco threat detection (ITDR) policy that raises alerts on matching events.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Policy ID (stable UUID, set after create).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title": schema.StringAttribute{
				Description: "Human-readable title (min 10 chars).",
				Required:    true,
			},
			attrDescription: schema.StringAttribute{
				Description: "Longer description shown in the Reco platform. " + writeOnlyNote,
				Optional:    true,
			},
			"data_source": schema.StringAttribute{
				Description: "Data source whose events the policy evaluates, e.g. GSUITE_ADMIN_AUDIT_LOG_API. " + writeOnlyNote,
				Required:    true,
			},
			"violation_risk_level": schema.StringAttribute{
				Description: "Severity of a generated alert: LOW, MEDIUM, HIGH, or CRITICAL. " + writeOnlyNote,
				Required:    true,
				Validators:  []validator.String{stringvalidator.OneOf(riskLevels...)},
			},
			"violation_risk_type": schema.StringAttribute{
				Description: "Risk category: RISK_TYPE_UNKNOWN, RISK_TYPE_DATA, RISK_TYPE_USER, RISK_TYPE_USER_TARGET, RISK_TYPE_USER_GROUP, or RISK_TYPE_APPLICATION. " + writeOnlyNote,
				Required:    true,
				Validators:  []validator.String{stringvalidator.OneOf(riskTypes...)},
			},
			"default_status": schema.StringAttribute{
				Description: "Initial status for new instances: POLICY_STATUS_OFF, POLICY_STATUS_PREVIEW, or POLICY_STATUS_ON. " + writeOnlyNote,
				Optional:    true,
				Validators:  []validator.String{stringvalidator.OneOf(policyStatuses...)},
			},
			"conditions_jsonata": schema.StringAttribute{
				Description: "JSONata expression that evaluates to true when an event violates the policy. " + writeOnlyNote,
				Required:    true,
			},
			"description_template": schema.StringAttribute{
				Description: "JSONata template producing the alert description (min 5 chars). " + writeOnlyNote,
				Required:    true,
			},
			"how_to_remediate": schema.StringAttribute{
				Description: "Remediation guidance shown to operators. " + writeOnlyNote,
				Optional:    true,
			},
			"why_should_i_care": schema.StringAttribute{
				Description: "Business context explaining why this policy matters. " + writeOnlyNote,
				Optional:    true,
			},
			"tags": schema.ListAttribute{
				Description: "Categorization tags.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"severity": schema.StringAttribute{
				Description: "Severity returned by the API (read-only).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Current policy status (read-only).",
				Computed:    true,
			},
			attrCreatedAt: schema.StringAttribute{
				Description: "Policy creation timestamp (read-only).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *threatDetectionPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &r.client, &resp.Diagnostics)
}

func (r *threatDetectionPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan threatDetectionPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	def := modelToTDPolicyDef(&plan)
	id, err := r.client.CreateThreatDetectionPolicy(ctx, &def)
	if err != nil {
		resp.Diagnostics.AddError("Error creating threat detection policy", err.Error())
		return
	}
	plan.ID = types.StringValue(id)

	full, err := r.client.GetThreatDetectionPolicyByID(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created threat detection policy", err.Error())
		return
	}
	tdPolicyToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *threatDetectionPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state threatDetectionPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.client.GetThreatDetectionPolicyByID(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading threat detection policy", err.Error())
		return
	}

	tdPolicyToModel(policy, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *threatDetectionPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan threatDetectionPolicyResourceModel
	var state threatDetectionPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID

	defUpd := modelToTDPolicyDef(&plan)
	if err := r.client.UpdateThreatDetectionPolicy(ctx, state.ID.ValueString(), &defUpd); err != nil {
		resp.Diagnostics.AddError("Error updating threat detection policy", err.Error())
		return
	}

	full, err := r.client.GetThreatDetectionPolicyByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated threat detection policy", err.Error())
		return
	}
	tdPolicyToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *threatDetectionPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state threatDetectionPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteThreatDetectionPolicy(ctx, state.ID.ValueString()); err != nil {
		if !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Error deleting threat detection policy", err.Error())
		}
	}
}

func (r *threatDetectionPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	policy, err := r.client.GetThreatDetectionPolicyByID(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing threat detection policy", fmt.Sprintf("cannot find policy %q: %s", req.ID, err))
		return
	}
	var state threatDetectionPolicyResourceModel
	state.ID = types.StringValue(policy.ID)
	tdPolicyToModel(policy, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func modelToTDPolicyDef(m *threatDetectionPolicyResourceModel) client.ThreatDetectionPolicyDefinition {
	def := client.ThreatDetectionPolicyDefinition{
		Title:               m.Title.ValueString(),
		Description:         m.Description.ValueString(),
		DataSource:          m.DataSource.ValueString(),
		ViolationRiskLevel:  m.ViolationRiskLevel.ValueString(),
		ViolationRiskType:   m.ViolationRiskType.ValueString(),
		DefaultStatus:       m.DefaultStatus.ValueString(),
		ConditionsJsonata:   m.ConditionsJsonata.ValueString(),
		DescriptionTemplate: m.DescriptionTemplate.ValueString(),
		HowToRemediate:      m.HowToRemediate.ValueString(),
		WhyShouldICare:      m.WhyShouldICare.ValueString(),
	}
	if !m.Tags.IsNull() && !m.Tags.IsUnknown() {
		def.Tags = tfutil.ListToStringSlice(m.Tags)
	}
	return def
}

func tdPolicyToModel(p *client.ThreatDetectionPolicy, m *threatDetectionPolicyResourceModel) {
	m.ID = types.StringValue(p.ID)
	m.Title = types.StringValue(p.Name)
	m.Severity = types.StringValue(p.Severity)
	m.Status = types.StringValue(p.Status)
	m.CreatedAt = tfutil.StringPtrValue(p.CreatedAt)
	m.Tags = tfutil.StringSliceToList(p.Tags)
}
