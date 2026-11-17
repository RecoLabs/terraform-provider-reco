package resources_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/acctestutil"
	"github.com/recolabs/terraform-provider-reco/internal/client"
)

func TestAccThreatDetectionExclusion_lifecycle(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := acctestutil.Suffix()
	policyTitle := fmt.Sprintf("tf-acc-itdr-excl-policy-%s", ts)
	name := fmt.Sprintf("tf-acc-exclusion-%s", ts)
	renamed := name + "-renamed"
	validUntil := "2099-01-01T00:00:00Z"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkThreatDetectionExclusionDestroyed(name),
			checkThreatDetectionExclusionDestroyed(renamed),
			checkThreatDetectionPolicyDestroyed(policyTitle),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccThreatDetectionExclusionConfig(policyTitle, name, client.ExclusionStateOnPermanent, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_threat_detection_exclusion.test", "name", name),
					resource.TestCheckResourceAttr("reco_threat_detection_exclusion.test", "state", client.ExclusionStateOnPermanent),
					resource.TestCheckResourceAttrSet("reco_threat_detection_exclusion.test", "id"),
					resource.TestCheckNoResourceAttr("reco_threat_detection_exclusion.test", "description"),
					resource.TestCheckNoResourceAttr("reco_threat_detection_exclusion.test", "valid_until"),
				),
			},
			{
				Config: testAccThreatDetectionExclusionConfig(policyTitle, renamed, client.ExclusionStateOnTemporary, validUntil),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_threat_detection_exclusion.test", "name", renamed),
					resource.TestCheckResourceAttr("reco_threat_detection_exclusion.test", "state", client.ExclusionStateOnTemporary),
					resource.TestCheckResourceAttrSet("reco_threat_detection_exclusion.test", "valid_until"),
				),
			},
			{
				Config: testAccThreatDetectionExclusionConfig(policyTitle, renamed, client.ExclusionStateOff, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_threat_detection_exclusion.test", "state", client.ExclusionStateOff),
					resource.TestCheckNoResourceAttr("reco_threat_detection_exclusion.test", "valid_until"),
				),
			},
		},
	})
}

func TestAccThreatDetectionExclusion_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := acctestutil.Suffix()
	policyTitle := fmt.Sprintf("tf-acc-itdr-excl-import-%s", ts)
	name := fmt.Sprintf("tf-acc-exclusion-import-%s", ts)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkThreatDetectionExclusionDestroyed(name),
			checkThreatDetectionPolicyDestroyed(policyTitle),
		),
		Steps: []resource.TestStep{
			{Config: testAccThreatDetectionExclusionConfig(policyTitle, name, client.ExclusionStateOnPermanent, "")},
			{
				ResourceName:      "reco_threat_detection_exclusion.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccThreatDetectionExclusionConfig(policyTitle, name, state, validUntil string) string {
	validUntilLine := ""
	if validUntil != "" {
		validUntilLine = fmt.Sprintf("\n  valid_until = %q", validUntil)
	}
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_threat_detection_policy" "test" {
  title                = %q
  description          = "Acceptance test policy for an exclusion"
  data_source          = "GSUITE_ADMIN_AUDIT_LOG_API"
  violation_risk_level = "LOW"
  violation_risk_type  = "RISK_TYPE_USER"
  default_status       = "POLICY_STATUS_OFF"
  conditions_jsonata   = "true"
  description_template = "\"Test alert\""
}

resource "reco_threat_detection_exclusion" "test" {
  name       = %q
  expression = "true"
  policy_ids = [reco_threat_detection_policy.test.id]
  state      = %q%s
}
`, policyTitle, name, state, validUntilLine)
}

func checkThreatDetectionExclusionDestroyed(name string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
		exclusions, err := c.ListAllExclusions(context.Background())
		if err != nil {
			return err
		}
		for i := range exclusions {
			if exclusions[i].Name == name {
				return fmt.Errorf("threat detection exclusion %q still exists after destroy", name)
			}
		}
		return nil
	}
}
