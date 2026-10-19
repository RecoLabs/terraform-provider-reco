package resources_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

func unitPolicyConfig(srv *fakereco.Server, title, level, status string) string {
	return unitProviderConfig(srv) + fmt.Sprintf(`
resource "reco_threat_detection_policy" "test" {
  title                = %q
  description          = "unit test policy"
  data_source          = "GSUITE_ADMIN_AUDIT_LOG_API"
  violation_risk_level = %q
  violation_risk_type  = "RISK_TYPE_USER"
  default_status       = %q
  conditions_jsonata   = "true"
  description_template = "\"Test alert\""
}
`, title, level, status)
}

func findPolicy(srv *fakereco.Server, title string) *client.ThreatDetectionPolicy {
	list := srv.Policies()
	for i := range list {
		if list[i].Name == title {
			return &list[i]
		}
	}
	return nil
}

func TestThreatDetectionPolicy_lifecycle(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{
			Config: unitPolicyConfig(srv, "Policy title one", "LOW", "POLICY_STATUS_OFF"),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("reco_threat_detection_policy.test", "id"),
				resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "title", "Policy title one"),
				resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "severity", "LOW"),
				resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "status", "POLICY_STATUS_OFF"),
				resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "tags.#", "0"),
				resource.TestCheckResourceAttrSet("reco_threat_detection_policy.test", "created_at"),
			),
		},
		resource.TestStep{
			Config:           unitPolicyConfig(srv, "Policy title renamed", "CRITICAL", "POLICY_STATUS_ON"),
			ConfigPlanChecks: expectAction("reco_threat_detection_policy.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "title", "Policy title renamed"),
				resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "severity", "CRITICAL"),
				resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "status", "POLICY_STATUS_ON"),
				func(*terraform.State) error {
					if p := findPolicy(srv, "Policy title renamed"); p == nil || len(srv.Policies()) != 1 {
						return fmt.Errorf("rename should update in place, API has %+v", srv.Policies())
					}
					return nil
				},
			),
		},
		resource.TestStep{
			ResourceName:      "reco_threat_detection_policy.test",
			ImportState:       true,
			ImportStateVerify: true,
			ImportStateVerifyIgnore: []string{
				"description", "data_source", "violation_risk_level", "violation_risk_type", "default_status",
				"conditions_jsonata", "description_template", "how_to_remediate", "why_should_i_care",
			},
		},
	)
	if n := len(srv.Policies()); n != 0 {
		t.Errorf("policy should be destroyed at the end of the test, %d left", n)
	}
}

func TestThreatDetectionPolicy_invalidEnumsRejectedAtPlanTime(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{
			Config:      unitPolicyConfig(srv, "Policy invalid", "SEVERE", "POLICY_STATUS_OFF"),
			PlanOnly:    true,
			ExpectError: errorMatching(`violation_risk_level[\s\S]*must be one of`),
		},
		resource.TestStep{
			Config:      unitPolicyConfig(srv, "Policy invalid", "LOW", "ENABLED"),
			PlanOnly:    true,
			ExpectError: errorMatching(`default_status[\s\S]*must be one of`),
		},
	)
}

func TestThreatDetectionPolicy_recreatedWhenDeletedOutOfBand(t *testing.T) {
	srv := fakereco.New(t)
	cfg := unitPolicyConfig(srv, "Policy drift one", "LOW", "POLICY_STATUS_OFF")
	unitTest(t,
		resource.TestStep{Config: cfg},
		resource.TestStep{
			PreConfig:        func() { srv.DeletePolicy(findPolicy(srv, "Policy drift one").ID) },
			Config:           cfg,
			ConfigPlanChecks: expectAction("reco_threat_detection_policy.test", plancheck.ResourceActionCreate),
			Check: func(*terraform.State) error {
				if findPolicy(srv, "Policy drift one") == nil {
					return fmt.Errorf("policy was not recreated")
				}
				return nil
			},
		},
	)
}

func TestThreatDetectionPolicy_createErrorIsReported(t *testing.T) {
	srv := fakereco.New(t)
	srv.FailWith("POST", "/policies/create", 400, "unknown data source")
	unitTest(t, resource.TestStep{
		Config:      unitPolicyConfig(srv, "Policy error", "LOW", "POLICY_STATUS_OFF"),
		ExpectError: errorMatching(`Error creating threat detection policy[\s\S]*unknown data source`),
	})
}

func TestThreatDetectionPolicy_importUnknownID(t *testing.T) {
	srv := fakereco.New(t)
	cfg := unitPolicyConfig(srv, "Policy import", "LOW", "POLICY_STATUS_OFF")
	unitTest(t,
		resource.TestStep{Config: cfg},
		resource.TestStep{
			Config:        cfg,
			ResourceName:  "reco_threat_detection_policy.test",
			ImportState:   true,
			ImportStateId: "policy-999",
			ExpectError:   errorMatching(`cannot find policy "policy-999"`),
		},
	)
}
