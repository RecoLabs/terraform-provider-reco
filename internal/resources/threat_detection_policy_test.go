package resources_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/client"
)

func TestAccThreatDetectionPolicy_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := testTimestamp()
	title := fmt.Sprintf("TF Acc ITDR Policy %d", ts)
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

func TestAccThreatDetectionPolicy_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := testTimestamp()
	title := fmt.Sprintf("TF Acc ITDR Import %d", ts)

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
