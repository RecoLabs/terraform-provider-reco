//nolint:goconst // tests repeat control IDs and field names on purpose
package resources

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
	"github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

var testControlAttrTypes = map[string]attr.Type{
	attrControlID:       types.StringType,
	attrTitle:           types.StringType,
	attrDescription:     types.StringType,
	attrParentControlID: types.StringType,
	attrRelatedPolicies: types.SetType{ElemType: types.StringType},
	attrManual:          types.BoolType,
	attrSkipped:         types.BoolType,
}

func testControl(t *testing.T, fields map[string]any) complianceControlModel {
	t.Helper()
	m := complianceControlModel{
		ControlID:        types.StringNull(),
		Title:            types.StringNull(),
		Description:      types.StringNull(),
		ParentControlID:  types.StringNull(),
		RelatedPolicyIDs: types.SetNull(types.StringType),
		Manual:           types.BoolNull(),
		Skipped:          types.BoolNull(),
	}
	for k, v := range fields {
		switch k {
		case attrControlID:
			m.ControlID = types.StringValue(v.(string))
		case attrTitle:
			m.Title = types.StringValue(v.(string))
		case attrDescription:
			m.Description = types.StringValue(v.(string))
		case attrParentControlID:
			m.ParentControlID = types.StringValue(v.(string))
		case attrRelatedPolicies:
			m.RelatedPolicyIDs = tfutil.StringSliceToSet(v.([]string))
		case attrManual:
			m.Manual = types.BoolValue(v.(bool))
		case attrSkipped:
			m.Skipped = types.BoolValue(v.(bool))
		default:
			t.Fatalf("unknown control field %q", k)
		}
	}
	return m
}

func testControlsList(t *testing.T, controls ...complianceControlModel) types.List {
	t.Helper()
	if controls == nil {
		// ListValueFrom turns a nil slice into a null list, not an empty one.
		controls = []complianceControlModel{}
	}
	list, diags := types.ListValueFrom(context.Background(), types.ObjectType{AttrTypes: testControlAttrTypes}, controls)
	if diags.HasError() {
		t.Fatalf("build controls list: %v", diags)
	}
	return list
}

func TestComplianceControlsFromList(t *testing.T) {
	policyA := "11111111-1111-4111-8111-111111111111"
	policyB := "22222222-2222-4222-8222-222222222222"

	tests := []struct {
		name string
		list types.List
		want []client.ComplianceControl
	}{
		{
			name: "null list has no controls",
			list: types.ListNull(types.ObjectType{AttrTypes: testControlAttrTypes}),
			want: nil,
		},
		{
			name: "unknown list has no controls",
			list: types.ListUnknown(types.ObjectType{AttrTypes: testControlAttrTypes}),
			want: nil,
		},
		{
			name: "empty list has an empty, non-nil result",
			list: testControlsList(t),
			want: []client.ComplianceControl{},
		},
		{
			name: "omitted optional fields become zero values",
			list: testControlsList(t, testControl(t, map[string]any{attrControlID: "A.1"})),
			want: []client.ComplianceControl{{ControlID: "A.1"}},
		},
		{
			name: "every field is carried over, in order",
			list: testControlsList(t,
				testControl(t, map[string]any{
					attrControlID:       "CC6",
					"title":             "Logical access",
					attrDescription:     "Parent control",
					attrRelatedPolicies: []string{policyA},
				}),
				testControl(t, map[string]any{
					attrControlID:       "CC6.1",
					attrParentControlID: "CC6",
					attrRelatedPolicies: []string{policyA, policyB},
					attrManual:          true,
					attrSkipped:         true,
				}),
			),
			want: []client.ComplianceControl{
				{ControlID: "CC6", Title: "Logical access", Description: "Parent control", RelatedPolicyIDs: []string{policyA}},
				{ControlID: "CC6.1", ParentControlID: "CC6", RelatedPolicyIDs: []string{policyA, policyB}, Manual: true, Skipped: true},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := complianceControlsFromList(context.Background(), tc.list)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			// A set has no order, so compare related policy IDs as sets.
			for i := range got {
				slices.Sort(got[i].RelatedPolicyIDs)
			}
			for i := range tc.want {
				slices.Sort(tc.want[i].RelatedPolicyIDs)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("controls = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestUpsertRequest_carriesFrameworkAndControls(t *testing.T) {
	model := &complianceFrameworkResourceModel{
		ID:          types.StringValue("fw-id"),
		Title:       types.StringValue("Title"),
		Description: types.StringValue("Desc"),
		Version:     types.StringValue("3"),
		Type:        types.StringValue("internal"),
		IsActive:    types.BoolValue(true),
	}
	controls := []client.ComplianceControl{{ControlID: "A.1"}}

	got := upsertRequest(model, controls)

	want := &client.UpsertComplianceFrameworkRequest{
		Framework: client.ComplianceFrameworkInput{
			ID: "fw-id", Title: "Title", Description: "Desc", Version: "3", Type: "internal", IsActive: true,
		},
		Controls: controls,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request = %+v, want %+v", got, want)
	}
}

func TestComplianceFrameworkToModel_leavesControlsAlone(t *testing.T) {
	controls := testControlsList(t, testControl(t, map[string]any{attrControlID: "A.1"}))
	model := &complianceFrameworkResourceModel{Controls: controls, Description: types.StringValue("stale")}

	complianceFrameworkToModel(&client.ComplianceFramework{
		ID: "fw-id", Title: "T", Description: "", Version: "2", Type: "custom", IsCustom: true, IsActive: true,
		CreatedAt: "2026-11-01T10:00:00Z", UpdatedAt: "2026-11-02T10:00:00Z",
	}, model)

	if !model.Controls.Equal(controls) {
		t.Errorf("controls changed on read: %v", model.Controls)
	}
	checks := map[string]bool{
		"id":          model.ID.Equal(types.StringValue("fw-id")),
		"title":       model.Title.Equal(types.StringValue("T")),
		"description": model.Description.Equal(types.StringValue("")),
		"version":     model.Version.Equal(types.StringValue("2")),
		"type":        model.Type.Equal(types.StringValue("custom")),
		"is_active":   model.IsActive.Equal(types.BoolValue(true)),
		"created_at":  model.CreatedAt.Equal(types.StringValue("2026-11-01T10:00:00Z")),
		"updated_at":  model.UpdatedAt.Equal(types.StringValue("2026-11-02T10:00:00Z")),
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("%s was not read from the API", name)
		}
	}
}

func TestComplianceFrameworkSchema_isValid(t *testing.T) {
	var resp resource.SchemaResponse
	NewComplianceFrameworkResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("invalid schema: %v", diags)
	}
}

func TestComplianceFrameworkMetadata(t *testing.T) {
	var resp resource.MetadataResponse
	NewComplianceFrameworkResource().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "reco"}, &resp)
	if resp.TypeName != "reco_compliance_framework" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestComplianceFrameworkValidateConfig_duplicateControlIDs(t *testing.T) {
	ctx := context.Background()
	r := &complianceFrameworkResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	tests := []struct {
		name     string
		controls types.List
		wantErrs []string
	}{
		{
			name:     "no controls",
			controls: types.ListNull(types.ObjectType{AttrTypes: testControlAttrTypes}),
		},
		{
			name: "unique IDs",
			controls: testControlsList(t,
				testControl(t, map[string]any{attrControlID: "A.1"}),
				testControl(t, map[string]any{attrControlID: "A.2"}),
			),
		},
		{
			name: "IDs are case sensitive",
			controls: testControlsList(t,
				testControl(t, map[string]any{attrControlID: "a.1"}),
				testControl(t, map[string]any{attrControlID: "A.1"}),
			),
		},
		{
			name: "duplicate ID is reported at the second control",
			controls: testControlsList(t,
				testControl(t, map[string]any{attrControlID: "A.1"}),
				testControl(t, map[string]any{attrControlID: "A.2"}),
				testControl(t, map[string]any{attrControlID: "A.1"}),
			),
			wantErrs: []string{`control_id "A.1" is already used by controls[0]`},
		},
		{
			name: "every extra duplicate is reported",
			controls: testControlsList(t,
				testControl(t, map[string]any{attrControlID: "A.1"}),
				testControl(t, map[string]any{attrControlID: "A.1"}),
				testControl(t, map[string]any{attrControlID: "A.1"}),
			),
			wantErrs: []string{"controls[0]", "controls[0]"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := tfsdk.State{
				Schema: schemaResp.Schema,
				Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
			}
			model := complianceFrameworkResourceModel{
				ID: types.StringNull(), Title: types.StringValue("T"), Description: types.StringNull(),
				Version: types.StringNull(), Type: types.StringNull(), IsActive: types.BoolNull(),
				Controls: tc.controls, CreatedAt: types.StringNull(), UpdatedAt: types.StringNull(),
			}
			if diags := state.Set(ctx, &model); diags.HasError() {
				t.Fatalf("build config: %v", diags)
			}

			resp := &resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}}, resp)

			errs := errorDetails(resp.Diagnostics)
			if len(errs) != len(tc.wantErrs) {
				t.Fatalf("errors = %v, want %d matching %v", errs, len(tc.wantErrs), tc.wantErrs)
			}
			for i, want := range tc.wantErrs {
				if !strings.Contains(errs[i], want) {
					t.Errorf("error %d = %q, want it to contain %q", i, errs[i], want)
				}
			}
		})
	}
}

func TestComplianceFrameworkValidateConfig_errorPointsAtDuplicateControl(t *testing.T) {
	ctx := context.Background()
	r := &complianceFrameworkResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)}
	model := complianceFrameworkResourceModel{
		ID: types.StringNull(), Title: types.StringValue("T"), Description: types.StringNull(),
		Version: types.StringNull(), Type: types.StringNull(), IsActive: types.BoolNull(),
		Controls: testControlsList(t,
			testControl(t, map[string]any{attrControlID: "A.1"}),
			testControl(t, map[string]any{attrControlID: "A.1"}),
		),
		CreatedAt: types.StringNull(), UpdatedAt: types.StringNull(),
	}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("build config: %v", diags)
	}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}}, resp)

	if len(resp.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %v", resp.Diagnostics)
	}
	withPath, ok := resp.Diagnostics[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("diagnostic %T has no attribute path", resp.Diagnostics[0])
	}
	want := path.Root(attrControls).AtListIndex(1).AtName(attrControlID)
	if !withPath.Path().Equal(want) {
		t.Errorf("path = %s, want %s", withPath.Path(), want)
	}
}

func errorDetails(diags diag.Diagnostics) []string {
	errs := diags.Errors()
	out := make([]string, 0, len(errs))
	for _, d := range errs {
		out = append(out, d.Summary()+": "+d.Detail())
	}
	return out
}

func validateConfigErrors(t *testing.T, controls ...complianceControlModel) []string {
	t.Helper()
	ctx := context.Background()
	r := &complianceFrameworkResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)}
	model := complianceFrameworkResourceModel{
		ID: types.StringNull(), Title: types.StringValue("T"), Description: types.StringNull(),
		Version: types.StringNull(), Type: types.StringNull(), IsActive: types.BoolNull(),
		Controls: testControlsList(t, controls...), CreatedAt: types.StringNull(), UpdatedAt: types.StringNull(),
	}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("build config: %v", diags)
	}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}}, resp)
	return errorDetails(resp.Diagnostics)
}

func TestComplianceFrameworkValidateConfig_parentControls(t *testing.T) {
	control := func(id, parent string) complianceControlModel {
		fields := map[string]any{attrControlID: id}
		if parent != "" {
			fields[attrParentControlID] = parent
		}
		return testControl(t, fields)
	}
	tests := []struct {
		name     string
		controls []complianceControlModel
		want     string // substring of the only expected error, or "" for none
	}{
		{"no parents", []complianceControlModel{control("A", ""), control("B", "")}, ""},
		{"a valid tree", []complianceControlModel{control("A", ""), control("B", "A"), control("C", "B")}, ""},
		{"parent listed after its child", []complianceControlModel{control("B", "A"), control("A", "")}, ""},
		{"unknown parent", []complianceControlModel{control("A", "missing")}, `parent_control_id "missing" is not the control_id`},
		{"own parent", []complianceControlModel{control("A", "A")}, `control "A" cannot be its own parent_control_id`},
		{"two-control cycle", []complianceControlModel{control("A", "B"), control("B", "A")}, "parent_control_id cycle"},
		{"cycle that other controls hang off", []complianceControlModel{control("A", "B"), control("B", "A"), control("C", "A")}, "parent_control_id cycle"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateConfigErrors(t, tc.controls...)
			if tc.want == "" {
				if len(errs) != 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), tc.want) {
				t.Fatalf("errors = %v, want one containing %q", errs, tc.want)
			}
		})
	}
}

func TestInParentCycle_onlyControlsInTheCycle(t *testing.T) {
	parents := map[string]string{"D": "A", "A": "B", "B": "A", "E": ""}
	tests := map[string]bool{"A": true, "B": true, "D": false, "E": false}
	for control, want := range tests {
		if got := inParentCycle(control, parents); got != want {
			t.Errorf("inParentCycle(%q) = %v, want %v", control, got, want)
		}
	}
}

func TestComplianceFrameworkValidateConfig_cycleErrorsNameOnlyControlsInTheCycle(t *testing.T) {
	errs := validateConfigErrors(t,
		testControl(t, map[string]any{attrControlID: "A", attrParentControlID: "B"}),
		testControl(t, map[string]any{attrControlID: "B", attrParentControlID: "A"}),
		testControl(t, map[string]any{attrControlID: "D", attrParentControlID: "A"}),
	)
	if len(errs) != 2 {
		t.Fatalf("errors = %v, want one each for A and B", errs)
	}
	for _, err := range errs {
		if strings.Contains(err, `"D"`) {
			t.Errorf("control D only leads into the cycle but was reported as part of it: %s", err)
		}
	}
}

func TestValidateControlParents_waitsForUnknownValues(t *testing.T) {
	known := testControl(t, map[string]any{attrControlID: "B", attrParentControlID: "A"})
	unknownID := testControl(t, nil)
	unknownID.ControlID = types.StringUnknown()

	var diags diag.Diagnostics
	validateControlParents([]complianceControlModel{known, unknownID}, &diags)
	if diags.HasError() {
		t.Errorf("a parent that may be the control with an unknown ID was rejected: %v", diags)
	}
}

// If adding the controls fails, the framework exists without them, so the state saved for the tainted
// resource must not list controls that Reco does not have.
func TestComplianceFrameworkCreate_failedControlsAreNotSavedInState(t *testing.T) {
	ctx := context.Background()
	srv := fakereco.New(t)
	srv.FailWith("PUT", "/compliance-frameworks/upsert", 400, "injected failure")
	r := &complianceFrameworkResource{client: client.New(srv.URL, fakereco.APIKey, "test")}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)}
	model := complianceFrameworkResourceModel{
		ID: types.StringUnknown(), Title: types.StringValue("Create failure"), Description: types.StringUnknown(),
		Version: types.StringUnknown(), Type: types.StringUnknown(), IsActive: types.BoolUnknown(),
		Controls:  testControlsList(t, testControl(t, map[string]any{attrControlID: "A.1"})),
		CreatedAt: types.StringUnknown(), UpdatedAt: types.StringUnknown(),
	}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("build plan: %v", diags)
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)}}

	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the failed control upsert to be reported")
	}
	var saved complianceFrameworkResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatalf("read saved state: %v", diags)
	}
	if saved.ID.IsNull() || saved.ID.ValueString() == "" {
		t.Error("the created framework's ID must stay in state so the resource is tainted, not orphaned")
	}
	if !saved.Controls.IsNull() {
		t.Errorf("state lists controls %v that Reco does not have", saved.Controls)
	}
}
