package resources_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/client"
)

const (
	postureCheckResourceName = "reco_posture_check.test"
	postureCheckPresetID     = "1a6e62ff-be9c-4cb8-9c63-cc1dd6994927"
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
					resource.TestCheckResourceAttr(postureCheckResourceName, "title", title),
					resource.TestCheckResourceAttrSet(postureCheckResourceName, "id"),
					resource.TestCheckResourceAttr(postureCheckResourceName, "default_status", "POLICY_STATUS_OFF"),
					resource.TestCheckResourceAttr(postureCheckResourceName, "conditions_jsonata", "true"),
					resource.TestCheckResourceAttr(postureCheckResourceName, "entities.#", "1"),
					resource.TestCheckResourceAttr(postureCheckResourceName, "entities.0.jsonata", "true"),
					resource.TestCheckNoResourceAttr(postureCheckResourceName, "affected_entities_columns"),
					resource.TestCheckNoResourceAttr(postureCheckResourceName, "affected_entities_preset_id"),
				),
			},
			{
				Config: testAccPostureCheckConfig(renamed),
				Check:  resource.TestCheckResourceAttr(postureCheckResourceName, "title", renamed),
			},
			{
				PreConfig:          func() { changeRemediationOutsideTerraform(t, renamed) },
				Config:             testAccPostureCheckConfig(renamed),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func changeRemediationOutsideTerraform(t *testing.T, title string) {
	t.Helper()
	ctx := context.Background()
	c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
	checks, err := c.ListAllPostureChecks(ctx)
	if err != nil {
		t.Fatalf("list posture checks: %v", err)
	}
	for i := range checks {
		if checks[i].Name != title {
			continue
		}
		def, err := c.GetPostureCheckDefinition(ctx, checks[i].ID)
		if err != nil {
			t.Fatalf("get posture check: %v", err)
		}
		def.HowToRemediate = "Changed outside Terraform."
		if err := c.UpdatePostureCheck(ctx, checks[i].ID, def); err != nil {
			t.Fatalf("update posture check: %v", err)
		}
		return
	}
	t.Fatalf("posture check %q not found", title)
}

func TestAccPostureCheck_entities(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	title := fmt.Sprintf("TF Acc Posture Entities %d", testTimestamp())
	oneColumn := `[{ name = "Is Admin", jsonata = "payload.isAdmin" }]`
	twoColumns := `[
    { name = "Is Admin", jsonata = "payload.isAdmin" },
    { name = "Email", jsonata = "payload.email" },
  ]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkPostureCheckDestroyed(title),
		Steps: []resource.TestStep{
			{
				Config: testAccPostureCheckEntitiesConfig(title, oneColumn),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(postureCheckResourceName, "entities.#", "2"),
					resource.TestCheckResourceAttr(postureCheckResourceName, "entities.0.name", "Active accounts"),
					resource.TestCheckResourceAttr(postureCheckResourceName, "entities.1.name", ""),
					resource.TestCheckResourceAttr(postureCheckResourceName, "conditions_jsonata", "(payload.active = true) and (payload.isAdmin = true)"),
					resource.TestCheckResourceAttr(postureCheckResourceName, "affected_entities_preset_id", postureCheckPresetID),
					resource.TestCheckResourceAttr(postureCheckResourceName, "affected_entities_columns.#", "1"),
					resource.TestCheckResourceAttr(postureCheckResourceName, "default_status", "POLICY_STATUS_ON"),
				),
			},
			{
				Config: testAccPostureCheckEntitiesConfig(title, twoColumns),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(postureCheckResourceName, "affected_entities_columns.#", "2"),
					resource.TestCheckResourceAttr(postureCheckResourceName, "affected_entities_columns.1.name", "Email"),
				),
			},
			{
				ResourceName:      postureCheckResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccPostureCheck_invalidConditions(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	testCases := []struct {
		desc       string
		conditions string
		wantError  string
	}{{
		desc:       "both conditions_jsonata and entities",
		conditions: `conditions_jsonata = "true"` + "\n" + `entities = [{ jsonata = "true" }]`,
		wantError:  "Invalid Attribute Combination",
	}, {
		desc:       "neither conditions_jsonata nor entities",
		conditions: "",
		wantError:  "Missing Attribute Configuration",
	}, {
		desc:       "entity without jsonata",
		conditions: `entities = [{ name = "Admins" }]`,
		wantError:  "Invalid Attribute Combination",
	}}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: providerFactories(),
				Steps: []resource.TestStep{{
					Config: testAccProviderConfig() + fmt.Sprintf(`
resource "reco_posture_check" "test" {
  title                  = "TF Acc Posture Invalid"
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "LOW"
  violation_risk_type    = "RISK_TYPE_USER"
  posture_unique_jsonata = "$.id"
  posture_value_jsonata  = "$.value"
  %s
}
`, tC.conditions),
					ExpectError: regexp.MustCompile(tC.wantError),
				}},
			})
		})
	}
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
				ResourceName:      postureCheckResourceName,
				ImportState:       true,
				ImportStateVerify: true,
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
  how_to_remediate       = "Fix it."
  why_should_i_care      = "It matters."
  tags                   = ["terraform-acc"]
}
`, title)
}

func testAccPostureCheckEntitiesConfig(title, columns string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_posture_check" "test" {
  title                  = %q
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "MEDIUM"
  violation_risk_type    = "RISK_TYPE_USER"
  posture_unique_jsonata = "$.id"
  posture_value_jsonata  = "$.value"
  status_jsonata         = "true_count > 0"
  entities = [
    { name = "Active accounts", jsonata = "payload.active = true" },
    { jsonata = "payload.isAdmin = true" },
  ]
  affected_entities_preset_id = %q
  affected_entities_columns   = %s
}
`, title, postureCheckPresetID, columns)
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
