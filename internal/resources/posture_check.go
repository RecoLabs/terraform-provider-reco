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
	_ resource.Resource                = &postureCheckResource{}
	_ resource.ResourceWithImportState = &postureCheckResource{}
	_ resource.ResourceWithConfigure   = &postureCheckResource{}
)

type postureCheckResource struct {
	client *client.Client
}

func NewPostureCheckResource() resource.Resource {
	return &postureCheckResource{}
}

type postureCheckResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Title                types.String `tfsdk:"title"`
	Description          types.String `tfsdk:"description"`
	DataSource           types.String `tfsdk:"data_source"`
	ViolationRiskLevel   types.String `tfsdk:"violation_risk_level"`
	ViolationRiskType    types.String `tfsdk:"violation_risk_type"`
	DefaultStatus        types.String `tfsdk:"default_status"`
	PostureUniqueJsonata types.String `tfsdk:"posture_unique_jsonata"`
	PostureValueJsonata  types.String `tfsdk:"posture_value_jsonata"`
	ConditionsJsonata    types.String `tfsdk:"conditions_jsonata"`
	StatusJsonata        types.String `tfsdk:"status_jsonata"`
	HowToRemediate       types.String `tfsdk:"how_to_remediate"`
	WhyShouldICare       types.String `tfsdk:"why_should_i_care"`
	Tags                 types.List   `tfsdk:"tags"`
	Severity             types.String `tfsdk:"severity"`
	AppSource            types.String `tfsdk:"app_source"`
}

func (r *postureCheckResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_posture_check"
}

func (r *postureCheckResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Reco posture check policy.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Posture check ID (stable UUID, set after create).",
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
				Description: "Longer description shown in the Reco UI.",
				Optional:    true,
				Computed:    true,
			},
			"data_source": schema.StringAttribute{
				Description: "Data source the check evaluates, e.g. GSUITE_USERS_API. " + writeOnlyNote,
				Required:    true,
			},
			"violation_risk_level": schema.StringAttribute{
				Description: "Severity of a failing posture issue: LOW, MEDIUM, HIGH, or CRITICAL. " + writeOnlyNote,
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
			"posture_unique_jsonata": schema.StringAttribute{
				Description: "JSONata expression that uniquely identifies each tracked entity (min 2 chars), e.g. $.primaryEmail. " + writeOnlyNote,
				Required:    true,
			},
			"posture_value_jsonata": schema.StringAttribute{
				Description: "JSONata expression returning the display value for each tracked entity (min 2 chars), e.g. $.primaryEmail. " + writeOnlyNote,
				Required:    true,
			},
			"conditions_jsonata": schema.StringAttribute{
				Description: "JSONata expression that evaluates to true when an entity violates the check. " + writeOnlyNote,
				Required:    true,
			},
			"status_jsonata": schema.StringAttribute{
				Description: "JSONata expression evaluated over the results that returns true when the check fails. " + writeOnlyNote,
				Optional:    true,
			},
			"how_to_remediate": schema.StringAttribute{
				Description: "Remediation guidance shown to operators. " + writeOnlyNote,
				Optional:    true,
			},
			"why_should_i_care": schema.StringAttribute{
				Description: "Business context explaining why this check matters. " + writeOnlyNote,
				Optional:    true,
			},
			"tags": schema.ListAttribute{
				Description: "Categorization tags.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"severity": schema.StringAttribute{
				Description: "Severity level returned by the API (read-only).",
				Computed:    true,
			},
			"app_source": schema.StringAttribute{
				Description: "Source application identifier (read-only).",
				Computed:    true,
			},
		},
	}
}

func (r *postureCheckResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &r.client, &resp.Diagnostics)
}

func (r *postureCheckResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan postureCheckResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	def := modelToPostureCheckDef(&plan)
	id, err := r.client.CreatePostureCheck(ctx, &def)
	if err != nil {
		resp.Diagnostics.AddError("Error creating posture check", err.Error())
		return
	}
	plan.ID = types.StringValue(id)

	full, err := r.client.GetPostureCheckByID(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created posture check", err.Error())
		return
	}
	postureCheckToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *postureCheckResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state postureCheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	check, err := r.client.GetPostureCheckByID(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading posture check", err.Error())
		return
	}

	postureCheckToModel(check, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *postureCheckResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan postureCheckResourceModel
	var state postureCheckResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID

	defUpd := modelToPostureCheckDef(&plan)
	if err := r.client.UpdatePostureCheck(ctx, state.ID.ValueString(), &defUpd); err != nil {
		resp.Diagnostics.AddError("Error updating posture check", err.Error())
		return
	}

	full, err := r.client.GetPostureCheckByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated posture check", err.Error())
		return
	}
	postureCheckToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *postureCheckResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state postureCheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeletePostureCheck(ctx, state.ID.ValueString()); err != nil {
		if !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Error deleting posture check", err.Error())
		}
	}
}

func (r *postureCheckResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	check, err := r.client.GetPostureCheckByID(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing posture check", fmt.Sprintf("cannot find posture check %q: %s", req.ID, err))
		return
	}
	var state postureCheckResourceModel
	state.ID = types.StringValue(check.ID)
	postureCheckToModel(check, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func modelToPostureCheckDef(m *postureCheckResourceModel) client.PostureCheckDefinition {
	def := client.PostureCheckDefinition{
		Title:                m.Title.ValueString(),
		Description:          m.Description.ValueString(),
		DataSource:           m.DataSource.ValueString(),
		ViolationRiskLevel:   m.ViolationRiskLevel.ValueString(),
		ViolationRiskType:    m.ViolationRiskType.ValueString(),
		DefaultStatus:        m.DefaultStatus.ValueString(),
		PostureUniqueJsonata: m.PostureUniqueJsonata.ValueString(),
		PostureValueJsonata:  m.PostureValueJsonata.ValueString(),
		ConditionsJsonata:    m.ConditionsJsonata.ValueString(),
		StatusJsonata:        m.StatusJsonata.ValueString(),
		HowToRemediate:       m.HowToRemediate.ValueString(),
		WhyShouldICare:       m.WhyShouldICare.ValueString(),
	}
	if !m.Tags.IsNull() && !m.Tags.IsUnknown() {
		def.Tags = tfutil.ListToStringSlice(m.Tags)
	}
	return def
}

func postureCheckToModel(c *client.PostureCheck, m *postureCheckResourceModel) {
	m.ID = types.StringValue(c.ID)
	m.Title = types.StringValue(c.Name)
	m.Description = types.StringValue(c.Description)
	m.Severity = types.StringValue(c.Severity)
	m.AppSource = types.StringValue(c.AppSource)
	m.Tags = tfutil.StringSliceToList(c.Tags)
}
