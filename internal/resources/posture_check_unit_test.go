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
	return unitProviderConfig(srv) + fmt.Sprintf(`
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
	for _, c := range srv.PostureChecks() {
		if c.Name == title {
			return &c
		}
	}
	return nil
}

// writeOnlyPostureAttrs are sent to the API but never returned by it, so they cannot round-trip through import.
var writeOnlyPostureAttrs = []string{
	"data_source", "violation_risk_level", "violation_risk_type", "default_status",
	"conditions_jsonata", "status_jsonata", "posture_unique_jsonata", "posture_value_jsonata",
	"how_to_remediate", "why_should_i_care",
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
			ResourceName:            "reco_posture_check.test",
			ImportState:             true,
			ImportStateVerify:       true,
			ImportStateVerifyIgnore: writeOnlyPostureAttrs,
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
