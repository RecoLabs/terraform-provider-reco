package resources_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/acctestutil"
	"github.com/recolabs/terraform-provider-reco/internal/client"
)

// The tests below run against a real tenant (TF_ACC, RECO_BASE_URL and
// RECO_API_KEY). The API key's role needs PERM_COMPLIANCE_CREATE, _READ,
// _UPDATE and _DELETE, and PERM_POLICIES_* for the posture check they map.
// compliance_framework_unit_test.go covers the same lifecycle against the
// in-memory fake API, without a tenant.

const complianceFrameworkResourceName = "reco_compliance_framework.test"

func testAccComplianceClient() *client.Client {
	return client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
}

// listPageSize gives each list call its own page size so the API's list cache cannot serve it (see
// uncachedComplianceFilter in the client).
var listPageSize atomic.Int64

func testAccUncachedClient() *client.Client {
	return testAccComplianceClient().WithPageSize(101 + listPageSize.Add(1)%800)
}

func TestAccComplianceFramework_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	suffix := acctestutil.Suffix()
	title := fmt.Sprintf("tf-acc-compliance-framework-%s", suffix)
	checkTitle := fmt.Sprintf("tf-acc-compliance-check-%s", suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkComplianceFrameworkDestroyed(title),
			checkPostureCheckDestroyed(checkTitle),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccComplianceFrameworkConfig(title, checkTitle, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(complianceFrameworkResourceName, "id"),
					resource.TestCheckResourceAttr(complianceFrameworkResourceName, "title", title),
					resource.TestCheckResourceAttr(complianceFrameworkResourceName, "description", "Acceptance test framework"),
					resource.TestCheckResourceAttr(complianceFrameworkResourceName, "version", "1.0"),
					resource.TestCheckResourceAttr(complianceFrameworkResourceName, "type", "internal"),
					resource.TestCheckResourceAttr(complianceFrameworkResourceName, "is_active", "false"),
					resource.TestCheckResourceAttrSet(complianceFrameworkResourceName, "created_at"),
					resource.TestCheckResourceAttr(complianceFrameworkResourceName, "controls.#", "2"),
					checkComplianceFrameworkIsCustom(),
					// The controls are write-only in Terraform, so the state alone cannot show
					// they reached Reco. The posture check lists the frameworks that map it.
					checkPostureCheckFrameworks(checkTitle, title, true),
				),
			},
			{
				Config:   testAccComplianceFrameworkConfig(title, checkTitle, true),
				PlanOnly: true,
			},
			{
				// Dropping the mapping from the control removes it from Reco.
				Config: testAccComplianceFrameworkConfig(title, checkTitle, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(complianceFrameworkResourceName, plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(complianceFrameworkResourceName, "controls.1.related_policy_ids.#", "0"),
					checkPostureCheckFrameworks(checkTitle, title, false),
				),
			},
			{
				ResourceName:            complianceFrameworkResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"controls"},
			},
		},
	})
}

func TestAccComplianceFramework_isActiveForcesReplacement(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	title := fmt.Sprintf("tf-acc-compliance-active-%s", acctestutil.Suffix())
	config := func(isActive bool) string {
		return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title     = %q
  is_active = %t
}
`, title, isActive)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkComplianceFrameworkDestroyed(title),
		Steps: []resource.TestStep{
			{
				Config: config(true),
				Check:  resource.TestCheckResourceAttr(complianceFrameworkResourceName, "is_active", "true"),
			},
			{
				Config: config(false),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(complianceFrameworkResourceName, plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: resource.TestCheckResourceAttr(complianceFrameworkResourceName, "is_active", "false"),
			},
		},
	})
}

func TestAccComplianceFramework_driftIsDetectedAndRestored(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	title := fmt.Sprintf("tf-acc-compliance-drift-%s", acctestutil.Suffix())
	config := testAccProviderConfig() + fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title    = %q
  controls = [{ control_id = "A.1", title = "First" }]
}
`, title)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkComplianceFrameworkDestroyed(title),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig:          func() { renameComplianceFrameworkOutsideTerraform(t, title) },
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr(complianceFrameworkResourceName, "title", title),
			},
		},
	})
}

func TestAccComplianceFramework_importBuiltInFails(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	builtIn := findBuiltInComplianceFramework(t)
	title := fmt.Sprintf("tf-acc-compliance-import-%s", acctestutil.Suffix())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{{
			Config: testAccProviderConfig() + fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title = %q
}
`, title),
			ResourceName:  complianceFrameworkResourceName,
			ImportState:   true,
			ImportStateId: builtIn.ID,
			ExpectError:   regexp.MustCompile(`(?s)Cannot import a built-in compliance\s+framework`),
		}},
	})
}

func TestAccComplianceFramework_invalidConfig(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	testCases := []struct {
		desc      string
		body      string
		wantError string
	}{
		{
			desc:      "duplicate control IDs",
			body:      `title = "tf-acc-invalid"` + "\n" + `controls = [{ control_id = "A.1" }, { control_id = "A.1" }]`,
			wantError: "Duplicate control ID",
		},
		{
			desc:      "empty title",
			body:      `title = ""`,
			wantError: "UTF-8 character count must be between 1 and 256",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: providerFactories(),
				Steps: []resource.TestStep{{
					Config:      testAccProviderConfig() + "resource \"reco_compliance_framework\" \"test\" {\n" + tC.body + "\n}\n",
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(`(?s)` + tC.wantError),
				}},
			})
		})
	}
}

func testAccComplianceFrameworkConfig(title, checkTitle string, mapCheck bool) string {
	related := "[]"
	if mapCheck {
		related = "[reco_posture_check.test.id]"
	}
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_posture_check" "test" {
  title                  = %q
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "LOW"
  violation_risk_type    = "RISK_TYPE_USER"
  default_status         = "POLICY_STATUS_OFF"
  conditions_jsonata     = "true"
  posture_unique_jsonata = "$.id"
  posture_value_jsonata  = "$.value"
}

resource "reco_compliance_framework" "test" {
  title       = %q
  description = "Acceptance test framework"
  version     = "1.0"
  type        = "internal"
  controls = [
    { control_id = "CC6", title = "Logical access" },
    {
      control_id         = "CC6.1"
      title              = "Restrict access"
      parent_control_id  = "CC6"
      related_policy_ids = %s
    },
  ]
}
`, checkTitle, title, related)
}

func checkComplianceFrameworkIsCustom() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id := s.RootModule().Resources[complianceFrameworkResourceName].Primary.ID
		framework, err := testAccComplianceClient().GetComplianceFrameworkByID(context.Background(), id)
		if err != nil {
			return err
		}
		if !framework.IsCustom {
			return fmt.Errorf("framework %q is not custom", id)
		}
		return nil
	}
}

// checkPostureCheckFrameworks asserts whether Reco lists frameworkTitle among
// the frameworks mapped to the posture check titled checkTitle.
func checkPostureCheckFrameworks(checkTitle, frameworkTitle string, want bool) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		checks, err := testAccUncachedClient().ListAllPostureChecks(context.Background())
		if err != nil {
			return err
		}
		for i := range checks {
			if checks[i].Name != checkTitle {
				continue
			}
			if got := slices.Contains(checks[i].ComplianceFrameworks, frameworkTitle); got != want {
				return fmt.Errorf("posture check %q lists frameworks %v; framework %q listed = %t, want %t",
					checkTitle, checks[i].ComplianceFrameworks, frameworkTitle, got, want)
			}
			return nil
		}
		return fmt.Errorf("posture check %q not found", checkTitle)
	}
}

func checkComplianceFrameworkDestroyed(title string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		frameworks, err := testAccUncachedClient().ListAllComplianceFrameworks(context.Background())
		if err != nil {
			return err
		}
		for i := range frameworks {
			if frameworks[i].Title == title {
				return fmt.Errorf("compliance framework %q still exists after destroy", title)
			}
		}
		return nil
	}
}

// renameComplianceFrameworkOutsideTerraform keeps the framework's other fields
// but not its controls, which the API replaces on every upsert and cannot read.
func renameComplianceFrameworkOutsideTerraform(t *testing.T, title string) {
	t.Helper()
	ctx := context.Background()
	c := testAccUncachedClient()
	frameworks, err := c.ListAllComplianceFrameworks(ctx)
	if err != nil {
		t.Fatalf("list compliance frameworks: %v", err)
	}
	for i := range frameworks {
		if frameworks[i].Title != title {
			continue
		}
		err := c.UpsertComplianceFramework(ctx, &client.UpsertComplianceFrameworkRequest{
			Framework: client.ComplianceFrameworkInput{
				ID: frameworks[i].ID, Title: title + " (renamed)", Description: frameworks[i].Description,
				Version: frameworks[i].Version, Type: frameworks[i].Type,
			},
			Controls: []client.ComplianceControl{{ControlID: "A.1", Title: "First"}},
		})
		if err != nil {
			t.Fatalf("rename compliance framework: %v", err)
		}
		return
	}
	t.Fatalf("compliance framework %q not found", title)
}

func findBuiltInComplianceFramework(t *testing.T) client.ComplianceFramework {
	t.Helper()
	frameworks, err := testAccUncachedClient().ListAllComplianceFrameworks(context.Background())
	if err != nil {
		t.Fatalf("list compliance frameworks: %v", err)
	}
	for _, framework := range frameworks {
		if !framework.IsCustom {
			return framework
		}
	}
	t.Skip("tenant has no built-in compliance framework to try importing")
	return client.ComplianceFramework{}
}
