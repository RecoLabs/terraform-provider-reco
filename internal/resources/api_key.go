package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.Resource                = &apiKeyResource{}
	_ resource.ResourceWithImportState = &apiKeyResource{}
	_ resource.ResourceWithConfigure   = &apiKeyResource{}
)

type apiKeyResource struct {
	client *client.Client
}

func NewApiKeyResource() resource.Resource {
	return &apiKeyResource{}
}

type apiKeyResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Role         types.String `tfsdk:"role"`
	PermittedIps types.List   `tfsdk:"permitted_ips"`
	Expiration   types.String `tfsdk:"expiration"`
	Secret       types.String `tfsdk:"secret"`
	CreatedBy    types.String `tfsdk:"created_by"`
	State        types.String `tfsdk:"state"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

func (r *apiKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Reco API key. The secret is returned only on creation and stored in Terraform state (sensitive). " +
			"Changing name, role, permitted_ips, or state updates the key in place. " +
			"state defaults to APPLICATION_KEY_STATE_ACTIVE. Set APPLICATION_KEY_STATE_RETIRED to soft-retire the key without deleting it or rotating its secret. " +
			"An empty permitted_ips list clears IP restrictions.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "API key ID (stable identifier, set after create).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			attrName: schema.StringAttribute{
				Description: "Human-readable key name (min 3 chars, must be unique within the tenant).",
				Required:    true,
			},
			"role": schema.StringAttribute{
				Description: "Role name assigned to this key.",
				Required:    true,
			},
			"permitted_ips": schema.ListAttribute{
				Description: "Optional IP allowlist. An empty list means unrestricted. Omit to keep unrestricted.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"expiration": schema.StringAttribute{
				Description: "Optional expiry timestamp in RFC3339 format.",
				Optional:    true,
			},
			"secret": schema.StringAttribute{
				Description: "API key secret. Shown only once on creation — stored in Terraform state. Handle with care.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_by": schema.StringAttribute{
				Description: "Email of the user who created this key (read-only).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"state": schema.StringAttribute{
				Description: "Lifecycle state: APPLICATION_KEY_STATE_ACTIVE (default) or APPLICATION_KEY_STATE_RETIRED. " +
					"Updated in place. Retiring keeps the resource and secret. Omit to keep the key active.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString(applicationKeyStateActive),
				Validators: []validator.String{stringvalidator.OneOf(applicationKeyStates...)},
			},
			attrCreatedAt: schema.StringAttribute{
				Description: "Creation timestamp (read-only).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *apiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tfutil.SetClient(req.ProviderData, &r.client, &resp.Diagnostics)
}

func (r *apiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateApiKeyRequest{
		Name:         plan.Name.ValueString(),
		Role:         plan.Role.ValueString(),
		PermittedIps: tfutil.ListToStringSlice(plan.PermittedIps),
	}
	if !plan.Expiration.IsNull() && !plan.Expiration.IsUnknown() {
		s := plan.Expiration.ValueString()
		createReq.Expiration = &s
	}

	created, err := r.client.CreateApiKey(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating API key", err.Error())
		return
	}

	plan.ID = types.StringValue(created.Key.ID)
	plan.Secret = types.StringValue(created.Secret)
	key := created.Key
	desiredState := apiKeyState(plan.State)
	if key.State != desiredState {
		updated, updateErr := r.client.UpdateApiKey(ctx, &client.UpdateApiKeyRequest{
			ID:           key.ID,
			Name:         key.Name,
			Role:         key.Role,
			PermittedIps: key.PermittedIps,
			State:        desiredState,
		})
		if updateErr != nil {
			resp.Diagnostics.AddError("Error updating API key state", updateErr.Error())
		} else {
			key = *updated
		}
	}
	apiKeyToModel(&key, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := r.client.GetApiKeyByID(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading API key", err.Error())
		return
	}

	apiKeyToModel(key, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan apiKeyResourceModel
	var state apiKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID

	updateReq := client.UpdateApiKeyRequest{
		ID:           state.ID.ValueString(),
		Name:         plan.Name.ValueString(),
		Role:         plan.Role.ValueString(),
		PermittedIps: tfutil.ListToStringSlice(plan.PermittedIps),
		State:        apiKeyState(plan.State),
	}
	updated, err := r.client.UpdateApiKey(ctx, &updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating API key", err.Error())
		return
	}

	apiKeyToModel(updated, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteApiKey(ctx, state.ID.ValueString()); err != nil {
		if !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Error deleting API key", err.Error())
		}
	}
}

func (r *apiKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	key, err := r.client.GetApiKeyByID(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing API key", fmt.Sprintf("cannot find API key %q: %s", req.ID, err))
		return
	}
	var state apiKeyResourceModel
	state.Secret = types.StringValue("")
	apiKeyToModel(key, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func apiKeyState(state types.String) string {
	if state.IsNull() || state.IsUnknown() || state.ValueString() == "" {
		return applicationKeyStateActive
	}
	return state.ValueString()
}

func apiKeyToModel(k *client.ApiKey, m *apiKeyResourceModel) {
	m.ID = types.StringValue(k.ID)
	m.Name = types.StringValue(k.Name)
	m.Role = types.StringValue(k.Role)
	m.CreatedBy = types.StringValue(k.CreatedBy)
	m.State = types.StringValue(k.State)
	m.CreatedAt = tfutil.StringPtrValue(k.CreatedAt)
	m.PermittedIps = tfutil.StringSliceToList(k.PermittedIps)
	// The API reports "no expiration" as the Unix epoch.
	if k.Expiration != nil && *k.Expiration != "" && *k.Expiration != "1970-01-01T00:00:00Z" {
		m.Expiration = types.StringValue(*k.Expiration)
	}
}
