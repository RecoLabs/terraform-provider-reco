//nolint:goconst // tests repeat resource addresses and API field names on purpose
package resources_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

const frameworkAddr = "reco_compliance_framework.test"

func frameworkConfig(srv *fakereco.Server, body string) string {
	return srv.ProviderConfig() + body
}

func findFramework(srv *fakereco.Server, title string) *client.ComplianceFramework {
	list := srv.ComplianceFrameworks()
	for i := range list {
		if list[i].IsCustom && list[i].Title == title {
			return &list[i]
		}
	}
	return nil
}

// checkControlIDs asserts the controls Reco holds for the framework titled title, which the provider
// cannot read back, so the state alone would pass even if nothing had been sent.
func checkControlIDs(srv *fakereco.Server, title string, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		fw := findFramework(srv, title)
		if fw == nil {
			return fmt.Errorf("framework %q not found in the API", title)
		}
		got := make([]string, 0)
		for _, control := range srv.ComplianceControls(fw.ID) {
			got = append(got, control.ControlID)
		}
		if !slices.Equal(got, want) {
			return fmt.Errorf("controls Reco holds for %q = %v, want %v", title, got, want)
		}
		return nil
	}
}

func customFrameworkCount(srv *fakereco.Server) int {
	n := 0
	for _, fw := range srv.ComplianceFrameworks() {
		if fw.IsCustom {
			n++
		}
	}
	return n
}

func lifecycleConfigs(srv *fakereco.Server, title, checkID, otherCheckID string) (create, updated string) {
	create = frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title       = %q
  description = "Created by a test"
  version     = "1.0"
  type        = "internal"
  controls = [
    { control_id = "CC6", title = "Logical access" },
    {
      control_id         = "CC6.1"
      title              = "Restrict access"
      description        = "Only authorised users"
      parent_control_id  = "CC6"
      related_policy_ids = [%q, %q]
      manual             = true
    },
  ]
}
`, title, checkID, otherCheckID))
	updated = frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title       = %q
  description = "Updated by a test"
  version     = "1.1"
  type        = "internal"
  controls = [
    { control_id = "CC6.1", title = "Restrict access", skipped = true, related_policy_ids = [%q] },
    { control_id = "CC7" },
  ]
}
`, title, checkID))
	return create, updated
}

func TestComplianceFramework_lifecycle(t *testing.T) {
	srv := fakereco.New(t)
	title := "Unit Framework"
	const checkID, otherCheckID = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	srv.SeedPostureCheckWithID(checkID, client.PostureCheck{Name: "Unit posture check", Severity: "LOW", AppSource: "GSUITE_USERS_API"})
	srv.SeedPostureCheckWithID(otherCheckID, client.PostureCheck{Name: "Other posture check", Severity: "LOW", AppSource: "GSUITE_USERS_API"})

	create, updated := lifecycleConfigs(srv, title, checkID, otherCheckID)

	var frameworkID string
	unitTest(t,
		resource.TestStep{
			Config: create,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet(frameworkAddr, "id"),
				resource.TestCheckResourceAttr(frameworkAddr, "title", title),
				resource.TestCheckResourceAttr(frameworkAddr, "description", "Created by a test"),
				resource.TestCheckResourceAttr(frameworkAddr, "version", "1.0"),
				resource.TestCheckResourceAttr(frameworkAddr, "type", "internal"),
				resource.TestCheckResourceAttr(frameworkAddr, "is_active", "false"),
				resource.TestCheckResourceAttrSet(frameworkAddr, "created_at"),
				resource.TestCheckResourceAttr(frameworkAddr, "controls.#", "2"),
				resource.TestCheckResourceAttr(frameworkAddr, "controls.1.related_policy_ids.#", "2"),
				checkControlIDs(srv, title, "CC6", "CC6.1"),
				func(s *terraform.State) error {
					frameworkID = s.RootModule().Resources[frameworkAddr].Primary.ID
					for _, c := range srv.ComplianceControls(frameworkID) {
						if c.ControlID != "CC6.1" {
							continue
						}
						if c.ParentControlID != "CC6" || !c.Manual || c.Skipped || c.Description != "Only authorised users" || len(c.RelatedPolicyIDs) != 2 {
							return fmt.Errorf("control CC6.1 stored in Reco = %+v", c)
						}
					}
					return nil
				},
				func(*terraform.State) error {
					for _, check := range srv.PostureChecks() {
						if !slices.Contains(check.ComplianceFrameworks, title) {
							return fmt.Errorf("posture check %q does not list the framework: %v", check.Name, check.ComplianceFrameworks)
						}
					}
					return nil
				},
			),
		},
		resource.TestStep{Config: create, PlanOnly: true}, // nothing to change after the refresh
		resource.TestStep{
			Config:           updated,
			ConfigPlanChecks: expectAction(frameworkAddr, plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(frameworkAddr, "description", "Updated by a test"),
				resource.TestCheckResourceAttr(frameworkAddr, "version", "1.1"),
				resource.TestCheckResourceAttr(frameworkAddr, "controls.#", "2"),
				checkControlIDs(srv, title, "CC6.1", "CC7"),
				func(s *terraform.State) error {
					if id := s.RootModule().Resources[frameworkAddr].Primary.ID; id != frameworkID {
						return fmt.Errorf("id changed on update from %q to %q", frameworkID, id)
					}
					for _, check := range srv.PostureChecks() {
						mapped := slices.Contains(check.ComplianceFrameworks, title)
						if mapped != (check.Name == "Unit posture check") {
							return fmt.Errorf("posture check %q lists frameworks %v after the mapping change", check.Name, check.ComplianceFrameworks)
						}
					}
					return nil
				},
			),
		},
		resource.TestStep{
			// A metadata-only change must still send the controls, or Reco would delete them.
			Config: strings.Replace(updated, `"Updated by a test"`, `"Reworded"`, 1),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(frameworkAddr, "description", "Reworded"),
				checkControlIDs(srv, title, "CC6.1", "CC7"),
			),
		},
		resource.TestStep{
			// Removing the controls from the configuration removes them from the framework.
			Config: frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title       = %q
  description = "Reworded"
  version     = "1.1"
  type        = "internal"
}
`, title)),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(frameworkAddr, "controls.#", "0"),
				checkControlIDs(srv, title),
			),
		},
	)
	if n := customFrameworkCount(srv); n != 0 {
		t.Errorf("the framework should be destroyed at the end of the test, %d left", n)
	}
}

func TestComplianceFramework_withoutControlsDoesNotUpsert(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t, resource.TestStep{
		Config: frameworkConfig(srv, `
resource "reco_compliance_framework" "test" {
  title = "Unit Bare Framework"
}
`),
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr(frameworkAddr, "is_active", "false"),
			resource.TestCheckResourceAttr(frameworkAddr, "description", ""),
			resource.TestCheckNoResourceAttr(frameworkAddr, "controls"),
			checkControlIDs(srv, "Unit Bare Framework"),
			func(*terraform.State) error {
				if n := srv.CountRequests("PUT", "/compliance-frameworks/upsert"); n != 0 {
					return fmt.Errorf("create without controls made %d upsert request(s), want none", n)
				}
				return nil
			},
		),
	})
}

func TestComplianceFramework_isActive(t *testing.T) {
	srv := fakereco.New(t)
	title := "Unit Active Framework"
	config := func(isActive string) string {
		return frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title     = %q
  is_active = %s
}
`, title, isActive))
	}
	unset := frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title = %q
}
`, title))

	unitTest(t,
		resource.TestStep{
			Config: config("true"),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(frameworkAddr, "is_active", "true"),
				func(*terraform.State) error {
					if fw := findFramework(srv, title); fw == nil || !fw.IsActive {
						return fmt.Errorf("framework was not created active in Reco")
					}
					return nil
				},
			),
		},
		resource.TestStep{
			// The API only applies is_active on creation, so a change is a replacement.
			Config:           config("false"),
			ConfigPlanChecks: expectAction(frameworkAddr, plancheck.ResourceActionDestroyBeforeCreate),
			Check:            resource.TestCheckResourceAttr(frameworkAddr, "is_active", "false"),
		},
		resource.TestStep{
			// Toggled in the UI while is_active is left out of the configuration: no diff, state follows Reco.
			PreConfig: func() {
				fw := findFramework(srv, title)
				srv.ModifyComplianceFramework(fw.ID, func(f *client.ComplianceFramework) { f.IsActive = true })
			},
			Config:           unset,
			ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			Check:            resource.TestCheckResourceAttr(frameworkAddr, "is_active", "true"),
		},
	)
}

func TestComplianceFramework_drift(t *testing.T) {
	srv := fakereco.New(t)
	title := "Unit Drift Framework"
	config := frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title    = %q
  controls = [{ control_id = "A.1" }, { control_id = "A.2" }]
}
`, title))

	unitTest(t,
		resource.TestStep{Config: config},
		resource.TestStep{
			// Renamed in the UI: Terraform sees it.
			PreConfig: func() {
				srv.ModifyComplianceFramework(findFramework(srv, title).ID, func(f *client.ComplianceFramework) { f.Title += " (renamed)" })
			},
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		resource.TestStep{
			Config: config,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(frameworkAddr, "title", title),
				checkControlIDs(srv, title, "A.1", "A.2"),
			),
		},
		resource.TestStep{
			// Deleted in the UI: Terraform recreates it with its controls.
			PreConfig:        func() { srv.DeleteComplianceFramework(findFramework(srv, title).ID) },
			Config:           config,
			ConfigPlanChecks: expectAction(frameworkAddr, plancheck.ResourceActionCreate),
			Check:            checkControlIDs(srv, title, "A.1", "A.2"),
		},
	)
}

func TestComplianceFramework_import(t *testing.T) {
	srv := fakereco.New(t)
	title := "Unit Import Framework"
	id := srv.SeedComplianceFramework(
		client.ComplianceFramework{Title: title, Description: "Imported", Version: "2", Type: "internal"},
		[]client.ComplianceControl{{ControlID: "A.1"}},
	)
	config := frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title       = %q
  description = "Imported"
  version     = "2"
  type        = "internal"
}
`, title))

	unitTest(t,
		resource.TestStep{
			Config:             config,
			ResourceName:       frameworkAddr,
			ImportState:        true,
			ImportStateId:      id,
			ImportStatePersist: true,
		},
		resource.TestStep{
			// Nothing differs, so nothing is sent and the controls Reco holds stay.
			Config:   config,
			PlanOnly: true,
			Check:    checkControlIDs(srv, title, "A.1"),
		},
		resource.TestStep{
			// The first real change replaces the controls with the (empty) configured list.
			Config: strings.Replace(config, `"Imported"`, `"Changed"`, 1),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(frameworkAddr, "description", "Changed"),
				checkControlIDs(srv, title),
			),
		},
	)
}

func TestComplianceFramework_importErrors(t *testing.T) {
	srv := fakereco.New(t)
	config := frameworkConfig(srv, `
resource "reco_compliance_framework" "test" {
  title = "Unit Import Errors"
}
`)
	unitTest(t,
		resource.TestStep{
			Config:        config,
			ResourceName:  frameworkAddr,
			ImportState:   true,
			ImportStateId: fakereco.BuiltInFrameworkID,
			ExpectError:   errorMatching(`(?s)Cannot import a built-in compliance\s+framework`),
		},
		resource.TestStep{
			Config:        config,
			ResourceName:  frameworkAddr,
			ImportState:   true,
			ImportStateId: "00000000-0000-4000-8000-0000000000ff",
			ExpectError:   errorMatching(`(?s)cannot find compliance framework`),
		},
	)
}

func TestComplianceFramework_invalidConfig(t *testing.T) {
	srv := fakereco.New(t)
	tests := []struct {
		name, body, wantError string
	}{
		{"duplicate control IDs", `title = "T"` + "\n" + `controls = [{ control_id = "A.1" }, { control_id = "A.1" }]`, "Duplicate control ID"},
		{"control without an ID", `title = "T"` + "\n" + `controls = [{ title = "No ID" }]`, "control_id.*is required"},
		{"empty title", `title = ""`, "UTF-8 character count must be between 1 and 256"},
		{"title missing", `description = "No title"`, "title.*is required"},
		{"related policy that is not a UUID", `title = "T"` + "\n" + `controls = [{ control_id = "A.1", related_policy_ids = ["nope"] }]`, "must be a UUID"},
		{"unknown parent control", `title = "T"` + "\n" + `controls = [{ control_id = "A.1", parent_control_id = "missing" }]`, "Unknown parent control"},
		{"control that is its own parent", `title = "T"` + "\n" + `controls = [{ control_id = "A.1", parent_control_id = "A.1" }]`, "Control is its own parent"},
		{
			"parent cycle",
			`title = "T"` + "\n" + `controls = [{ control_id = "A", parent_control_id = "B" }, { control_id = "B", parent_control_id = "A" }]`,
			"Parent control cycle",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			unitTest(t, resource.TestStep{
				Config:      frameworkConfig(srv, "resource \"reco_compliance_framework\" \"test\" {\n"+tc.body+"\n}\n"),
				PlanOnly:    true,
				ExpectError: errorMatching(`(?s)` + tc.wantError),
			})
		})
	}
	if n := customFrameworkCount(srv); n != 0 {
		t.Errorf("invalid configs created %d framework(s) in Reco", n)
	}
}

func TestComplianceFramework_apiErrors(t *testing.T) {
	missingCheck := "11111111-1111-4111-8111-111111111111"

	t.Run("create is rejected", func(t *testing.T) {
		srv := fakereco.New(t)
		srv.FailWith("POST", "/compliance-frameworks/create", 403, "no permission")
		unitTest(t, resource.TestStep{
			Config:      frameworkConfig(srv, "resource \"reco_compliance_framework\" \"test\" {\n  title = \"T\"\n}\n"),
			ExpectError: errorMatching(`Error creating compliance framework`),
		})
	})

	t.Run("a control mapped to a missing posture check", func(t *testing.T) {
		srv := fakereco.New(t)
		unitTest(t, resource.TestStep{
			Config: frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title    = "T"
  controls = [{ control_id = "A.1", related_policy_ids = [%q] }]
}
`, missingCheck)),
			ExpectError: errorMatching(`(?s)Error adding controls to created compliance\s+framework.*does not exist`),
		})
		if n := customFrameworkCount(srv); n != 0 {
			t.Errorf("%d framework(s) left in Reco after the failed create was cleaned up", n)
		}
	})

	t.Run("update is rejected", func(t *testing.T) {
		srv := fakereco.New(t)
		config := func(title string) string {
			return frameworkConfig(srv, fmt.Sprintf("resource \"reco_compliance_framework\" \"test\" {\n  title = %q\n}\n", title))
		}
		unitTest(t,
			resource.TestStep{Config: config("Before")},
			resource.TestStep{
				PreConfig:   func() { srv.FailWith("PUT", "/compliance-frameworks/upsert", 403, "no permission") },
				Config:      config("After"),
				ExpectError: errorMatching(`Error updating compliance framework`),
			},
			resource.TestStep{PreConfig: srv.ClearFailures, Config: config("Before"), PlanOnly: true},
		)
	})

	t.Run("delete is rejected", func(t *testing.T) {
		srv := fakereco.New(t)
		config := frameworkConfig(srv, "resource \"reco_compliance_framework\" \"test\" {\n  title = \"T\"\n}\n")
		unitTest(t,
			resource.TestStep{Config: config},
			resource.TestStep{
				PreConfig:   func() { srv.FailWith("DELETE", "/compliance-frameworks/", 403, "no permission") },
				Config:      frameworkConfig(srv, ""),
				ExpectError: errorMatching(`Error deleting compliance framework`),
			},
			resource.TestStep{PreConfig: srv.ClearFailures, Config: config, PlanOnly: true},
		)
	})

	t.Run("read fails", func(t *testing.T) {
		srv := fakereco.New(t)
		config := frameworkConfig(srv, "resource \"reco_compliance_framework\" \"test\" {\n  title = \"T\"\n}\n")
		unitTest(t,
			resource.TestStep{Config: config},
			resource.TestStep{
				PreConfig:   func() { srv.FailWith("GET", "/compliance-frameworks/list", 403, "no permission") },
				Config:      config,
				ExpectError: errorMatching(`Error reading compliance framework`),
			},
			resource.TestStep{PreConfig: srv.ClearFailures, Config: config, PlanOnly: true},
		)
	})
}

// A framework whose controls could not be added is tracked in state, so the next apply replaces it
// instead of leaving an orphan in Reco.
func TestComplianceFramework_failedControlsLeaveTaintedResource(t *testing.T) {
	srv := fakereco.New(t)
	config := frameworkConfig(srv, `
resource "reco_compliance_framework" "test" {
  title    = "Unit Tainted Framework"
  controls = [{ control_id = "A.1" }]
}
`)
	unitTest(t,
		resource.TestStep{
			PreConfig:   func() { srv.FailWith("PUT", "/compliance-frameworks/upsert", 400, "injected failure") },
			Config:      config,
			ExpectError: errorMatching(`Error adding controls to created compliance framework`),
		},
		resource.TestStep{
			PreConfig:        srv.ClearFailures,
			Config:           config,
			ConfigPlanChecks: expectAction(frameworkAddr, plancheck.ResourceActionDestroyBeforeCreate),
			Check: resource.ComposeTestCheckFunc(
				checkControlIDs(srv, "Unit Tainted Framework", "A.1"),
				func(*terraform.State) error {
					if n := customFrameworkCount(srv); n != 1 {
						return fmt.Errorf("%d custom framework(s) in Reco, want exactly 1 (no orphan)", n)
					}
					return nil
				},
			),
		},
	)
}

// The provider must see its own updates and out-of-band changes through the API's list cache.
func TestComplianceFramework_withCachedList(t *testing.T) {
	srv := fakereco.New(t)
	srv.CacheComplianceList()
	title := "Unit Cached Framework"
	config := func(description string) string {
		return frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title       = %q
  description = %q
  controls    = [{ control_id = "A.1" }]
}
`, title, description))
	}

	unitTest(t,
		resource.TestStep{Config: config("first")},
		resource.TestStep{
			Config: config("second"),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(frameworkAddr, "description", "second"),
				checkControlIDs(srv, title, "A.1"),
			),
		},
		resource.TestStep{Config: config("second"), PlanOnly: true}, // the refresh sees the update, not a cached copy
		resource.TestStep{
			PreConfig: func() {
				srv.ModifyComplianceFramework(findFramework(srv, title).ID, func(f *client.ComplianceFramework) { f.Description = "edited in the UI" })
			},
			Config:             config("second"),
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		resource.TestStep{
			PreConfig:        func() { srv.DeleteComplianceFramework(findFramework(srv, title).ID) },
			Config:           config("second"),
			ConfigPlanChecks: expectAction(frameworkAddr, plancheck.ResourceActionCreate),
			Check:            checkControlIDs(srv, title, "A.1"),
		},
	)
}

// The API limits lengths in characters, so text that is long in bytes but short in characters is valid.
func TestComplianceFramework_lengthLimitsCountCharacters(t *testing.T) {
	srv := fakereco.New(t)
	description := strings.Repeat("ש", 3000) // 6000 bytes, under the 4096 character limit
	unitTest(t, resource.TestStep{
		Config: frameworkConfig(srv, fmt.Sprintf(`
resource "reco_compliance_framework" "test" {
  title       = "Unit Multibyte Framework"
  description = %q
}
`, description)),
		Check: resource.TestCheckResourceAttr(frameworkAddr, "description", description),
	})
}
