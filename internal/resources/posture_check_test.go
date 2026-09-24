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

func TestAccPostureCheck_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := testTimestamp()
	title := fmt.Sprintf("TF Acc Posture Check %d", ts)
	renamed := title + " Renamed"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkPostureCheckDestroyed(title),
			checkPostureCheckDestroyed(renamed),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccPostureCheckConfig(title),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_posture_check.test", "title", title),
					resource.TestCheckResourceAttrSet("reco_posture_check.test", "id"),
				),
			},
			{
				Config: testAccPostureCheckConfig(renamed),
				Check:  resource.TestCheckResourceAttr("reco_posture_check.test", "title", renamed),
			},
		},
	})
}

func TestAccPostureCheck_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := testTimestamp()
	title := fmt.Sprintf("TF Acc Posture Import %d", ts)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkPostureCheckDestroyed(title),
		Steps: []resource.TestStep{
			{Config: testAccPostureCheckConfig(title)},
			{
				ResourceName:      "reco_posture_check.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"data_source", "violation_risk_level", "violation_risk_type", "default_status",
					"conditions_jsonata", "status_jsonata", "posture_unique_jsonata", "posture_value_jsonata",
					"how_to_remediate", "why_should_i_care",
				},
			},
		},
	})
}

func testAccPostureCheckConfig(title string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_posture_check" "test" {
  title                  = %q
  description            = "Acceptance test posture check"
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "LOW"
  violation_risk_type    = "RISK_TYPE_USER"
  default_status         = "POLICY_STATUS_OFF"
  conditions_jsonata     = "true"
  status_jsonata         = "false"
  posture_unique_jsonata = "$.id"
  posture_value_jsonata  = "$.value"
}
`, title)
}

func checkPostureCheckDestroyed(title string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
		checks, err := c.ListAllPostureChecks(context.Background())
		if err != nil {
			return err
		}
		for i := range checks {
			if checks[i].Name == title {
				return fmt.Errorf("posture check %q still exists after destroy", title)
			}
		}
		return nil
	}
}
