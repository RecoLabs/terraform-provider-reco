//nolint:goconst // tests repeat resource addresses and API field names on purpose
package resources_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/acctestutil"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

func TestAccThreatDetectionPolicy_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := acctestutil.Suffix()
	title := fmt.Sprintf("tf-acc-itdr-policy-%s", ts)
	renamed := title + " Renamed"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkThreatDetectionPolicyDestroyed(title),
			checkThreatDetectionPolicyDestroyed(renamed),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccThreatDetectionPolicyConfig(title),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "title", title),
					resource.TestCheckResourceAttrSet("reco_threat_detection_policy.test", "id"),
				),
			},
			{
				Config: testAccThreatDetectionPolicyConfig(renamed),
				Check:  resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "title", renamed),
			},
		},
	})
}

func TestAccThreatDetectionPolicy_instanceToStatus(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := acctestutil.Suffix()
	title := fmt.Sprintf("tf-acc-itdr-instances-%s", ts)
	const (
		offID     = "00000000-0000-4000-8000-000000000001"
		onID      = "00000000-0000-4000-8000-000000000002"
		previewID = "00000000-0000-4000-8000-000000000003"
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkThreatDetectionPolicyDestroyed(title),
		Steps: []resource.TestStep{
			{
				Config: testAccThreatDetectionPolicyInstanceConfig(title, map[string]string{
					offID:     "POLICY_STATUS_OFF",
					onID:      "POLICY_STATUS_ON",
					previewID: "POLICY_STATUS_PREVIEW",
				}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("reco_threat_detection_policy.test", "id"),
					resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "default_status", "POLICY_STATUS_PREVIEW"),
					resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "instance_to_status."+offID, "POLICY_STATUS_OFF"),
					resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "instance_to_status."+onID, "POLICY_STATUS_ON"),
					resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "instance_to_status."+previewID, "POLICY_STATUS_PREVIEW"),
				),
			},
		},
	})
}

func TestAccThreatDetectionPolicy_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := acctestutil.Suffix()
	title := fmt.Sprintf("tf-acc-itdr-import-%s", ts)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkThreatDetectionPolicyDestroyed(title),
		Steps: []resource.TestStep{
			{Config: testAccThreatDetectionPolicyConfig(title)},
			{
				ResourceName:      "reco_threat_detection_policy.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"description", "data_source", "violation_risk_level", "violation_risk_type", "default_status",
					"conditions_jsonata", "description_template", "how_to_remediate", "why_should_i_care",
				},
			},
		},
	})
}

func testAccThreatDetectionPolicyInstanceConfig(title string, status map[string]string) string {
	var entries strings.Builder
	for _, instanceID := range slices.Sorted(maps.Keys(status)) {
		fmt.Fprintf(&entries, "    %q = %q\n", instanceID, status[instanceID])
	}
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_threat_detection_policy" "test" {
  title                = %q
  description          = "Acceptance test threat detection policy"
  data_source          = "GSUITE_ADMIN_AUDIT_LOG_API"
  violation_risk_level = "LOW"
  violation_risk_type  = "RISK_TYPE_USER"
  default_status       = "POLICY_STATUS_PREVIEW"
  conditions_jsonata   = "true"
  description_template = "\"Test alert\""
  instance_to_status = {
%s  }
}
`, title, entries.String())
}

func testAccThreatDetectionPolicyConfig(title string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_threat_detection_policy" "test" {
  title                = %q
  description          = "Acceptance test threat detection policy"
  data_source          = "GSUITE_ADMIN_AUDIT_LOG_API"
  violation_risk_level = "LOW"
  violation_risk_type  = "RISK_TYPE_USER"
  default_status       = "POLICY_STATUS_OFF"
  conditions_jsonata   = "true"
  description_template = "\"Test alert\""
}
`, title)
}

func checkThreatDetectionPolicyDestroyed(title string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
		policies, err := c.ListAllPolicies(context.Background())
		if err != nil {
			return err
		}
		for i := range policies {
			if policies[i].Name == title {
				return fmt.Errorf("threat detection policy %q still exists after destroy", title)
			}
		}
		return nil
	}
}

func TestThreatDetectionPolicy_replacesWhenInstanceStatusChanges(t *testing.T) {
	srv := fakereco.New(t)
	const (
		offID = "instance-off"
		onID  = "instance-on"
	)
	created := unitPolicyConfigWithInstances(srv, "Policy instances", `
    "instance-off" = "POLICY_STATUS_OFF"
    "instance-on"  = "POLICY_STATUS_ON"`)
	removed := unitPolicyConfigWithInstances(srv, "Policy instances", `
    "instance-off" = "POLICY_STATUS_OFF"`)
	emptied := unitPolicyConfigWithInstances(srv, "Policy instances", "")
	omitted := unitPolicyConfig(srv, "Policy instances", "LOW", "POLICY_STATUS_PREVIEW")
	var policyID string
	unitTest(t,
		resource.TestStep{
			Config: created,
			Check: func(s *terraform.State) error {
				policyID = s.RootModule().Resources["reco_threat_detection_policy.test"].Primary.ID
				return expectInstanceStatus(srv, policyID, map[string]string{
					offID: "POLICY_STATUS_OFF",
					onID:  "POLICY_STATUS_ON",
				})
			},
		},
		resource.TestStep{
			Config:           removed,
			ConfigPlanChecks: expectAction("reco_threat_detection_policy.test", plancheck.ResourceActionReplace),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "instance_to_status."+offID, "POLICY_STATUS_OFF"),
				resource.TestCheckNoResourceAttr("reco_threat_detection_policy.test", "instance_to_status."+onID),
				func(s *terraform.State) error {
					nextID := s.RootModule().Resources["reco_threat_detection_policy.test"].Primary.ID
					if nextID == policyID {
						return fmt.Errorf("removing an override kept policy id %s", policyID)
					}
					if err := expectInstanceStatus(srv, policyID, map[string]string{}); err != nil {
						return err
					}
					policyID = nextID
					return expectInstanceStatus(srv, policyID, map[string]string{offID: "POLICY_STATUS_OFF"})
				},
			),
		},
		resource.TestStep{
			Config: removed,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
			},
		},
		resource.TestStep{
			Config:           emptied,
			ConfigPlanChecks: expectAction("reco_threat_detection_policy.test", plancheck.ResourceActionReplace),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_threat_detection_policy.test", "instance_to_status.%", "0"),
				func(s *terraform.State) error {
					nextID := s.RootModule().Resources["reco_threat_detection_policy.test"].Primary.ID
					if nextID == policyID {
						return fmt.Errorf("emptying the map kept policy id %s", policyID)
					}
					policyID = nextID
					return expectInstanceStatus(srv, policyID, map[string]string{})
				},
			),
		},
		resource.TestStep{
			Config:           omitted,
			ConfigPlanChecks: expectAction("reco_threat_detection_policy.test", plancheck.ResourceActionReplace),
			Check: func(s *terraform.State) error {
				nextID := s.RootModule().Resources["reco_threat_detection_policy.test"].Primary.ID
				if nextID == policyID {
					return fmt.Errorf("omitting the map kept policy id %s", policyID)
				}
				policyID = nextID
				return expectInstanceStatus(srv, policyID, map[string]string{})
			},
		},
		resource.TestStep{
			Config: omitted,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
			},
		},
	)
}
