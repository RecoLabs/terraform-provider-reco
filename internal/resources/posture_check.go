package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	tfutil "github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

var (
	_ resource.Resource                     = &postureCheckResource{}
	_ resource.ResourceWithImportState      = &postureCheckResource{}
	_ resource.ResourceWithConfigure        = &postureCheckResource{}
	_ resource.ResourceWithConfigValidators = &postureCheckResource{}
)

const (
	attrConditionsJsonata = "conditions_jsonata"
	attrEntities          = "entities"
	attrJsonata           = "jsonata"
)

var postureJsonataAttrTypes = map[string]attr.Type{
	attrName:    types.StringType,
	attrJsonata: types.StringType,
}

type postureCheckResource struct {
	client *client.Client
}

func NewPostureCheckResource() resource.Resource {
	return &postureCheckResource{}
}

type postureCheckResourceModel struct {
	ID                       types.String `tfsdk:"id"`
	Title                    types.String `tfsdk:"title"`
	Description              types.String `tfsdk:"description"`
	DataSource               types.String `tfsdk:"data_source"`
	ViolationRiskLevel       types.String `tfsdk:"violation_risk_level"`
	ViolationRiskType        types.String `tfsdk:"violation_risk_type"`
	DefaultStatus            types.String `tfsdk:"default_status"`
	PostureUniqueJsonata     types.String `tfsdk:"posture_unique_jsonata"`
	PostureValueJsonata      types.String `tfsdk:"posture_value_jsonata"`
	ConditionsJsonata        types.String `tfsdk:"conditions_jsonata"`
	Entities                 types.List   `tfsdk:"entities"`
	StatusJsonata            types.String `tfsdk:"status_jsonata"`
	AffectedEntitiesPresetID types.String `tfsdk:"affected_entities_preset_id"`
	AffectedEntitiesColumns  types.List   `tfsdk:"affected_entities_columns"`
	HowToRemediate           types.String `tfsdk:"how_to_remediate"`
	WhyShouldICare           types.String `tfsdk:"why_should_i_care"`
	Tags                     types.List   `tfsdk:"tags"`
	Severity                 types.String `tfsdk:"severity"`
	AppSource                types.String `tfsdk:"app_source"`
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
				Description: "Data source the check evaluates, e.g. GSUITE_USERS_API.",
				Required:    true,
			},
			"violation_risk_level": schema.StringAttribute{
				Description: "Severity of a failing posture issue: LOW, MEDIUM, HIGH, or CRITICAL.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.OneOf(riskLevels...)},
			},
			"violation_risk_type": schema.StringAttribute{
				Description: "Risk category: RISK_TYPE_UNKNOWN, RISK_TYPE_DATA, RISK_TYPE_USER, RISK_TYPE_USER_TARGET, " +
					"RISK_TYPE_USER_GROUP, or RISK_TYPE_APPLICATION.",
				Required:   true,
				Validators: []validator.String{stringvalidator.OneOf(riskTypes...)},
			},
			"default_status": schema.StringAttribute{
				Description: "Status for new instances: POLICY_STATUS_OFF, POLICY_STATUS_PREVIEW, or POLICY_STATUS_ON. " +
					"New posture checks start ON, so when this is set the provider applies it with an update right after create. " +
					"Read back from Reco; empty when instances have different statuses.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.OneOf(policyStatuses...)},
			},
			"posture_unique_jsonata": schema.StringAttribute{
				Description: "JSONata expression that uniquely identifies each tracked entity (min 2 chars), e.g. $.primaryEmail.",
				Required:    true,
			},
			"posture_value_jsonata": schema.StringAttribute{
				Description: "JSONata expression returning the display value for each tracked entity (min 2 chars), e.g. $.primaryEmail.",
				Required:    true,
			},
			attrConditionsJsonata: schema.StringAttribute{
				Description: "JSONata expression that evaluates to true when an entity violates the check. " +
					"Set exactly one of `conditions_jsonata` and `entities`; the other is read back from Reco.",
				Optional: true,
				Computed: true,
			},
			attrEntities: postureEntitiesAttribute(),
			"status_jsonata": schema.StringAttribute{
				Description: "JSONata expression evaluated over the results that returns true when the check fails.",
				Optional:    true,
			},
			"affected_entities_preset_id": schema.StringAttribute{
				Description: "ID of the preset column set for the affected-entities table (Posture Studio \"Preset Columns\").",
				Optional:    true,
			},
			"affected_entities_columns": postureColumnsAttribute(),
			"how_to_remediate": schema.StringAttribute{
				Description: "Remediation guidance shown to operators.",
				Optional:    true,
			},
			"why_should_i_care": schema.StringAttribute{
				Description: "Business context explaining why this check matters.",
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

// postureEntitiesAttribute keeps every nested attribute Computed: Terraform
// proposes null for an omitted Optional+Computed list whose prior value holds
// a non-computed attribute, which would plan an update after every apply that
// used conditions_jsonata. The object validator keeps jsonata required.
func postureEntitiesAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "Named conditions, AND-ed together, as in the Posture Studio Entities step. " +
			"Set exactly one of `conditions_jsonata` and `entities`; the other is read back from Reco. " +
			"`terraform plan -generate-config-out` writes both, so remove one from the generated configuration.",
		Optional: true,
		Computed: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				attrName: schema.StringAttribute{
					Description: "Display name shown in Posture Studio. The UI shows the JSONata when empty.",
					Optional:    true,
					Computed:    true,
					Default:     stringdefault.StaticString(""),
				},
				attrJsonata: schema.StringAttribute{
					Description: "Required. JSONata expression that must be true for the entity to be evaluated.",
					Optional:    true,
					Computed:    true,
				},
			},
			Validators: []validator.Object{
				objectvalidator.AlsoRequires(path.MatchRelative().AtName(attrJsonata)),
			},
		},
	}
}

func postureColumnsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "Extra columns of the affected-entities table (Posture Studio \"Additional Columns\").",
		Optional:    true,
		Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				attrName:    schema.StringAttribute{Required: true, Description: "Column header."},
				attrJsonata: schema.StringAttribute{Required: true, Description: "JSONata expression for the column value."},
			},
		},
	}
}

func (r *postureCheckResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(path.MatchRoot(attrConditionsJsonata), path.MatchRoot(attrEntities)),
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

	// Create ignores defaultStatus and starts the check ON; update applies it.
	if def.DefaultStatus != "" {
		if err := r.client.UpdatePostureCheck(ctx, id, &def); err != nil {
			resp.Diagnostics.AddError("Error applying default_status to created posture check", err.Error())
		}
	}

	if err := r.readInto(ctx, id, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading created posture check", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *postureCheckResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state postureCheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readInto(ctx, state.ID.ValueString(), &state); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading posture check", err.Error())
		return
	}
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

	if err := r.readInto(ctx, state.ID.ValueString(), &plan); err != nil {
		resp.Diagnostics.AddError("Error reading updated posture check", err.Error())
		return
	}
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
	var state postureCheckResourceModel
	if err := r.readInto(ctx, req.ID, &state); err != nil {
		resp.Diagnostics.AddError("Error importing posture check", fmt.Sprintf("cannot find posture check %q: %s", req.ID, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// readInto fills m from the full definition plus the list summary, which is
// the only source of severity and app_source.
func (r *postureCheckResource) readInto(ctx context.Context, id string, m *postureCheckResourceModel) error {
	def, err := r.client.GetPostureCheckDefinition(ctx, id)
	if err != nil {
		return err
	}
	summary, err := r.client.GetPostureCheckByID(ctx, id)
	if err != nil {
		return err
	}
	m.ID = types.StringValue(id)
	postureCheckDefToModel(def, m)
	m.Severity = types.StringValue(summary.Severity)
	m.AppSource = types.StringValue(summary.AppSource)
	return nil
}

func modelToPostureCheckDef(m *postureCheckResourceModel) client.PostureCheckDefinition {
	def := client.PostureCheckDefinition{
		Title:                    m.Title.ValueString(),
		Description:              m.Description.ValueString(),
		DataSource:               m.DataSource.ValueString(),
		ViolationRiskLevel:       m.ViolationRiskLevel.ValueString(),
		ViolationRiskType:        m.ViolationRiskType.ValueString(),
		DefaultStatus:            m.DefaultStatus.ValueString(),
		PostureUniqueJsonata:     m.PostureUniqueJsonata.ValueString(),
		PostureValueJsonata:      m.PostureValueJsonata.ValueString(),
		ConditionsJsonata:        m.ConditionsJsonata.ValueString(),
		StatusJsonata:            m.StatusJsonata.ValueString(),
		AffectedEntitiesPresetID: m.AffectedEntitiesPresetID.ValueString(),
		HowToRemediate:           m.HowToRemediate.ValueString(),
		WhyShouldICare:           m.WhyShouldICare.ValueString(),
	}
	if !m.Tags.IsNull() && !m.Tags.IsUnknown() {
		def.Tags = tfutil.ListToStringSlice(m.Tags)
	}
	for _, pair := range nameJsonataPairs(m.Entities) {
		def.Entities = append(def.Entities, client.PostureCheckEntity{Name: pair[0], Jsonata: pair[1]})
	}
	for _, pair := range nameJsonataPairs(m.AffectedEntitiesColumns) {
		def.AffectedEntitiesColumns = append(def.AffectedEntitiesColumns, client.PostureEntitiesColumn{Name: pair[0], Jsonata: pair[1]})
	}
	return def
}

func postureCheckDefToModel(d *client.PostureCheckDefinition, m *postureCheckResourceModel) {
	m.Title = types.StringValue(d.Title)
	m.Description = types.StringValue(d.Description)
	m.DataSource = types.StringValue(d.DataSource)
	m.ViolationRiskLevel = types.StringValue(d.ViolationRiskLevel)
	m.ViolationRiskType = types.StringValue(d.ViolationRiskType)
	m.DefaultStatus = stringOrNull(d.DefaultStatus)
	m.PostureUniqueJsonata = types.StringValue(d.PostureUniqueJsonata)
	m.PostureValueJsonata = types.StringValue(d.PostureValueJsonata)
	m.ConditionsJsonata = types.StringValue(d.ConditionsJsonata)
	m.StatusJsonata = stringOrNull(d.StatusJsonata)
	m.AffectedEntitiesPresetID = stringOrNull(d.AffectedEntitiesPresetID)
	m.HowToRemediate = stringOrNull(d.HowToRemediate)
	m.WhyShouldICare = stringOrNull(d.WhyShouldICare)
	m.Tags = tfutil.StringSliceToList(d.Tags)

	entities := make([][2]string, len(d.Entities))
	for i, entity := range d.Entities {
		entities[i] = [2]string{entity.Name, entity.Jsonata}
	}
	m.Entities = nameJsonataList(entities)
	m.AffectedEntitiesColumns = types.ListNull(types.ObjectType{AttrTypes: postureJsonataAttrTypes})
	if len(d.AffectedEntitiesColumns) > 0 {
		columns := make([][2]string, len(d.AffectedEntitiesColumns))
		for i, column := range d.AffectedEntitiesColumns {
			columns[i] = [2]string{column.Name, column.Jsonata}
		}
		m.AffectedEntitiesColumns = nameJsonataList(columns)
	}
}

// stringOrNull keeps optional attributes the API omits null, as they are in
// a config that does not set them.
func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func nameJsonataPairs(l types.List) [][2]string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	pairs := make([][2]string, 0, len(l.Elements()))
	for _, elem := range l.Elements() {
		obj, ok := elem.(types.Object)
		if !ok {
			continue
		}
		attrs := obj.Attributes()
		pairs = append(pairs, [2]string{
			attrs[attrName].(types.String).ValueString(),
			attrs[attrJsonata].(types.String).ValueString(),
		})
	}
	return pairs
}

func nameJsonataList(pairs [][2]string) types.List {
	elems := make([]attr.Value, len(pairs))
	for i, pair := range pairs {
		elems[i] = types.ObjectValueMust(postureJsonataAttrTypes, map[string]attr.Value{
			attrName:    types.StringValue(pair[0]),
			attrJsonata: types.StringValue(pair[1]),
		})
	}
	return types.ListValueMust(types.ObjectType{AttrTypes: postureJsonataAttrTypes}, elems)
}
