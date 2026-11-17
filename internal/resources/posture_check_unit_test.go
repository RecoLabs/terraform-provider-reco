//nolint:goconst // tests repeat resource addresses and API field names on purpose
package resources_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

func unitPostureCheckConfig(srv *fakereco.Server, title, level, extra string) string {
	return srv.ProviderConfig() + fmt.Sprintf(`
resource "reco_posture_check" "test" {
  title                  = %q
  description            = "unit test posture check"
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = %q
  violation_risk_type    = "RISK_TYPE_USER"
  default_status         = "POLICY_STATUS_OFF"
  conditions_jsonata     = "true"
  posture_unique_jsonata = "$.id"
  posture_value_jsonata  = "$.value"
  %s
}
`, title, level, extra)
}

func findPostureCheck(srv *fakereco.Server, title string) *client.PostureCheck {
	list := srv.PostureChecks()
	for i := range list {
		if list[i].Name == title {
			return &list[i]
		}
	}
	return nil
}

func TestPostureCheck_lifecycle(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{
			Config: unitPostureCheckConfig(srv, "Posture check one", "LOW", `tags = ["a", "b"]`),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("reco_posture_check.test", "id"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "title", "Posture check one"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "description", "unit test posture check"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "severity", "LOW"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "app_source", "GSUITE_USERS_API"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "tags.#", "2"),
			),
		},
		resource.TestStep{
			Config:           unitPostureCheckConfig(srv, "Posture check renamed", "HIGH", `tags = ["a"]`),
			ConfigPlanChecks: expectAction("reco_posture_check.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_posture_check.test", "title", "Posture check renamed"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "severity", "HIGH"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "tags.#", "1"),
				func(*terraform.State) error {
					if c := findPostureCheck(srv, "Posture check renamed"); c == nil || len(srv.PostureChecks()) != 1 {
						return fmt.Errorf("rename should update in place, API has %+v", srv.PostureChecks())
					}
					return nil
				},
			),
		},
		resource.TestStep{
			ResourceName:      "reco_posture_check.test",
			ImportState:       true,
			ImportStateVerify: true,
		},
	)
	if n := len(srv.PostureChecks()); n != 0 {
		t.Errorf("posture check should be destroyed at the end of the test, %d left", n)
	}
}

func TestPostureCheck_sendsFullDefinition(t *testing.T) {
	srv := fakereco.New(t)
	extra := `
  status_jsonata   = "false"
  how_to_remediate = "do the thing"
  why_should_i_care = "because"`
	unitTest(t, resource.TestStep{
		Config: unitPostureCheckConfig(srv, "Posture definition", "CRITICAL", extra),
		Check: func(*terraform.State) error {
			c := findPostureCheck(srv, "Posture definition")
			if c == nil || c.Severity != "CRITICAL" || c.AppSource != "GSUITE_USERS_API" {
				return fmt.Errorf("API-side posture check does not reflect the config: %+v", c)
			}
			return nil
		},
	})
}

func TestPostureCheck_invalidEnumsRejectedAtPlanTime(t *testing.T) {
	srv := fakereco.New(t)
	valid := unitPostureCheckConfig(srv, "Posture invalid", "LOW", "")
	unitTest(t,
		resource.TestStep{
			Config:      unitPostureCheckConfig(srv, "Posture invalid", "SEVERE", ""),
			PlanOnly:    true,
			ExpectError: errorMatching(`violation_risk_level[\s\S]*must be one of`),
		},
		resource.TestStep{
			Config:      strings.Replace(valid, `"RISK_TYPE_USER"`, `"RISK_TYPE_BOGUS"`, 1),
			PlanOnly:    true,
			ExpectError: errorMatching(`violation_risk_type[\s\S]*must be one of`),
		},
		resource.TestStep{
			Config:      strings.Replace(valid, `"POLICY_STATUS_OFF"`, `"ENABLED"`, 1),
			PlanOnly:    true,
			ExpectError: errorMatching(`default_status[\s\S]*must be one of`),
		},
	)
}

func TestPostureCheck_recreatedWhenDeletedOutOfBand(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitPostureCheckConfig(srv, "Posture drift one", "LOW", "")},
		resource.TestStep{
			PreConfig:        func() { srv.DeletePostureCheck(findPostureCheck(srv, "Posture drift one").ID) },
			Config:           unitPostureCheckConfig(srv, "Posture drift one", "LOW", ""),
			ConfigPlanChecks: expectAction("reco_posture_check.test", plancheck.ResourceActionCreate),
			Check: func(*terraform.State) error {
				if findPostureCheck(srv, "Posture drift one") == nil {
					return fmt.Errorf("posture check was not recreated")
				}
				return nil
			},
		},
	)
}

func TestPostureCheck_createErrorIsReported(t *testing.T) {
	srv := fakereco.New(t)
	srv.FailWith("POST", "/posture-checks/create", 400, "title must be at least 10 characters")
	unitTest(t, resource.TestStep{
		Config:      unitPostureCheckConfig(srv, "short", "LOW", ""),
		ExpectError: errorMatching(`Error creating posture check[\s\S]*at least 10 characters`),
	})
}

func TestPostureCheck_importUnknownID(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitPostureCheckConfig(srv, "Posture import", "LOW", "")},
		resource.TestStep{
			Config:        unitPostureCheckConfig(srv, "Posture import", "LOW", ""),
			ResourceName:  "reco_posture_check.test",
			ImportState:   true,
			ImportStateId: "posture-999",
			ExpectError:   errorMatching(`cannot find posture check "posture-999"`),
		},
	)
}

func unitPostureEntitiesConfig(srv *fakereco.Server, title, columns string) string {
	return srv.ProviderConfig() + fmt.Sprintf(`
resource "reco_posture_check" "test" {
  title                  = %q
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "MEDIUM"
  violation_risk_type    = "RISK_TYPE_USER"
  posture_unique_jsonata = "$.id"
  posture_value_jsonata  = "$.value"
  entities = [
    { name = "Active accounts", jsonata = "payload.active = true" },
    { jsonata = "payload.isAdmin = true" },
  ]
  affected_entities_preset_id = "preset-1"
  affected_entities_columns   = %s
}
`, title, columns)
}

func TestPostureCheck_entitiesAndAffectedColumns(t *testing.T) {
	srv := fakereco.New(t)
	one := `[{ name = "Is Admin", jsonata = "payload.isAdmin" }]`
	two := `[
    { name = "Is Admin", jsonata = "payload.isAdmin" },
    { name = "Email", jsonata = "payload.email" },
  ]`
	unitTest(t,
		resource.TestStep{
			Config: unitPostureEntitiesConfig(srv, "Posture entities one", one),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_posture_check.test", "entities.#", "2"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "entities.0.name", "Active accounts"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "entities.1.jsonata", "payload.isAdmin = true"),
				// The API folds the entities into the conditions.
				resource.TestCheckResourceAttr("reco_posture_check.test", "conditions_jsonata",
					"(payload.active = true) and (payload.isAdmin = true)"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "affected_entities_preset_id", "preset-1"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "affected_entities_columns.#", "1"),
			),
		},
		resource.TestStep{
			Config:           unitPostureEntitiesConfig(srv, "Posture entities one", two),
			ConfigPlanChecks: expectAction("reco_posture_check.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_posture_check.test", "affected_entities_columns.#", "2"),
				resource.TestCheckResourceAttr("reco_posture_check.test", "affected_entities_columns.1.name", "Email"),
			),
		},
		resource.TestStep{ResourceName: "reco_posture_check.test", ImportState: true, ImportStateVerify: true},
	)
}

func TestPostureCheck_plainConditionsBecomeOneEntity(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t, resource.TestStep{
		Config: unitPostureCheckConfig(srv, "Posture plain conditions", "LOW", ""),
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr("reco_posture_check.test", "conditions_jsonata", "true"),
			resource.TestCheckResourceAttr("reco_posture_check.test", "entities.#", "1"),
			resource.TestCheckResourceAttr("reco_posture_check.test", "entities.0.jsonata", "true"),
			resource.TestCheckNoResourceAttr("reco_posture_check.test", "affected_entities_columns"),
			resource.TestCheckNoResourceAttr("reco_posture_check.test", "affected_entities_preset_id"),
		),
	})
}

func TestPostureCheck_conditionsAndEntitiesAreMutuallyExclusive(t *testing.T) {
	srv := fakereco.New(t)
	base := func(conditions string) string {
		return srv.ProviderConfig() + fmt.Sprintf(`
resource "reco_posture_check" "test" {
  title                  = "Posture invalid"
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "LOW"
  violation_risk_type    = "RISK_TYPE_USER"
  posture_unique_jsonata = "$.id"
  posture_value_jsonata  = "$.value"
  %s
}
`, conditions)
	}
	unitTest(t,
		resource.TestStep{
			Config:      base(`conditions_jsonata = "true"` + "\n" + `entities = [{ jsonata = "true" }]`),
			PlanOnly:    true,
			ExpectError: errorMatching(`Invalid Attribute Combination`),
		},
		resource.TestStep{
			Config:      base(""),
			PlanOnly:    true,
			ExpectError: errorMatching(`Missing Attribute Configuration`),
		},
		resource.TestStep{
			Config:      base(`entities = [{ name = "Admins" }]`),
			PlanOnly:    true,
			ExpectError: errorMatching(`Invalid Attribute Combination`),
		},
	)
}

func TestPostureCheck_complianceFrameworks(t *testing.T) {
	srv := fakereco.New(t)
	const (
		frameworkA = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
		frameworkB = "6ba7b811-9dad-11d1-80b4-00c04fd430c8"
		frameworkC = "6ba7b812-9dad-11d1-80b4-00c04fd430c8"
	)
	withIDs := func(ids ...string) string {
		quoted := make([]string, len(ids))
		for i, id := range ids {
			quoted[i] = fmt.Sprintf("%q", id)
		}
		return "compliance_framework_ids = [" + strings.Join(quoted, ", ") + "]"
	}
	var createdID string
	unitTest(t,
		resource.TestStep{
			Config: unitPostureFrameworkConfig(srv, "Posture frameworks", withIDs(frameworkA, frameworkB)),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_posture_check.test", "compliance_framework_ids.#", "2"),
				resource.TestCheckTypeSetElemAttr("reco_posture_check.test", "compliance_framework_ids.*", frameworkA),
				resource.TestCheckTypeSetElemAttr("reco_posture_check.test", "compliance_framework_ids.*", frameworkB),
				func(state *terraform.State) error {
					createdID = state.RootModule().Resources["reco_posture_check.test"].Primary.ID
					return nil
				},
			),
		},
		resource.TestStep{
			Config:           unitPostureFrameworkConfig(srv, "Posture frameworks", withIDs(frameworkC)),
			ConfigPlanChecks: expectAction("reco_posture_check.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				func(state *terraform.State) error {
					got := state.RootModule().Resources["reco_posture_check.test"].Primary.ID
					if got != createdID {
						return fmt.Errorf("framework update replaced %s with %s", createdID, got)
					}
					if n := len(srv.PostureChecks()); n != 1 {
						return fmt.Errorf("framework update replaced the check, %d exist", n)
					}
					return nil
				},
				resource.TestCheckResourceAttr("reco_posture_check.test", "compliance_framework_ids.#", "1"),
				resource.TestCheckTypeSetElemAttr("reco_posture_check.test", "compliance_framework_ids.*", frameworkC),
			),
		},
		resource.TestStep{
			Config:           unitPostureFrameworkConfig(srv, "Posture frameworks", `compliance_framework_ids = []`),
			ConfigPlanChecks: expectAction("reco_posture_check.test", plancheck.ResourceActionUpdate),
			Check:            resource.TestCheckResourceAttr("reco_posture_check.test", "compliance_framework_ids.#", "0"),
		},
	)
	unitTest(t, resource.TestStep{
		Config: unitPostureFrameworkConfig(srv, "Posture frameworks omitted", ""),
		Check:  resource.TestCheckResourceAttr("reco_posture_check.test", "compliance_framework_ids.#", "0"),
	})
	unitTest(t, resource.TestStep{
		Config:      unitPostureFrameworkConfig(srv, "Posture frameworks invalid", `compliance_framework_ids = ["not-a-uuid"]`),
		PlanOnly:    true,
		ExpectError: errorMatching(`Invalid Attribute Value Match[\s\S]*must be a UUID`),
	})
}

func unitPostureFrameworkConfig(srv *fakereco.Server, title, frameworks string) string {
	return srv.ProviderConfig() + fmt.Sprintf(`
resource "reco_posture_check" "test" {
  title                  = %q
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "LOW"
  violation_risk_type    = "RISK_TYPE_USER"
  conditions_jsonata     = "true"
  posture_unique_jsonata = "$.id"
  posture_value_jsonata  = "$.value"
  %s
}
`, title, frameworks)
}

func TestPostureCheck_editsMadeOutsideTerraformAreDetected(t *testing.T) {
	srv := fakereco.New(t)
	cfg := unitPostureCheckConfig(srv, "Posture drift remediation", "LOW", `how_to_remediate = "original"`)
	unitTest(t,
		resource.TestStep{Config: cfg},
		resource.TestStep{
			PreConfig: func() {
				srv.ModifyPostureCheck(findPostureCheck(srv, "Posture drift remediation").ID, func(d *client.PostureCheckDefinition) {
					d.HowToRemediate = "changed in the UI"
				})
			},
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		// Applying the config puts it back (the framework fails the step if a plan after apply is not empty).
		resource.TestStep{Config: cfg},
	)
}
