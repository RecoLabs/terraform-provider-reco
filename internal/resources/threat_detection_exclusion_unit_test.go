package resources_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

const exclusionAddr = "reco_threat_detection_exclusion.test"

func unitGlobalExclusionConfig(srv *fakereco.Server, name, state, validUntil, description string) string {
	descriptionLine := ""
	if description != "" {
		descriptionLine = fmt.Sprintf("\n  description = %q", description)
	}
	validUntilLine := ""
	if validUntil != "" {
		validUntilLine = fmt.Sprintf("\n  valid_until = %q", validUntil)
	}
	stateLine := ""
	if state != "" {
		stateLine = fmt.Sprintf("\n  state = %q", state)
	}
	return srv.ProviderConfig() + fmt.Sprintf(`
resource "reco_threat_detection_exclusion" "test" {
  name       = %q
  expression = "true"
  is_global  = true%s%s%s
}
`, name, descriptionLine, stateLine, validUntilLine)
}

func unitScopedExclusionConfig(srv *fakereco.Server, name, state, validUntil string) string {
	validUntilLine := ""
	if validUntil != "" {
		validUntilLine = fmt.Sprintf("\n  valid_until = %q", validUntil)
	}
	return srv.ProviderConfig() + fmt.Sprintf(`
resource "reco_threat_detection_policy" "test" {
  title                = "Policy for exclusion"
  description          = "unit test policy"
  data_source          = "GSUITE_ADMIN_AUDIT_LOG_API"
  violation_risk_level = "LOW"
  violation_risk_type  = "RISK_TYPE_USER"
  default_status       = "POLICY_STATUS_OFF"
  conditions_jsonata   = "true"
  description_template = "\"Test alert\""
}

resource "reco_threat_detection_exclusion" "test" {
  name       = %q
  expression = "actor = \"svc\""
  policy_ids = [reco_threat_detection_policy.test.id]
  state      = %q%s
}
`, name, state, validUntilLine)
}

func findExclusion(srv *fakereco.Server, name string) *client.ThreatDetectionExclusion {
	list := srv.Exclusions()
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

func countStateCalls(srv *fakereco.Server) int {
	n := 0
	for _, req := range srv.Requests() {
		if strings.Contains(req, "/state") {
			n++
		}
	}
	return n
}

func TestThreatDetectionExclusion_omitOptionalFieldsDoesNotDiff(t *testing.T) {
	srv := fakereco.New(t)
	cfg := unitGlobalExclusionConfig(srv, "Trusted actor", "", "", "")
	unitTest(t,
		resource.TestStep{
			Config: cfg,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet(exclusionAddr, "id"),
				resource.TestCheckResourceAttr(exclusionAddr, "name", "Trusted actor"),
				resource.TestCheckResourceAttr(exclusionAddr, "state", client.ExclusionStateOnPermanent),
				resource.TestCheckResourceAttr(exclusionAddr, "is_global", "true"),
				resource.TestCheckNoResourceAttr(exclusionAddr, "description"),
				resource.TestCheckNoResourceAttr(exclusionAddr, "valid_until"),
				resource.TestCheckNoResourceAttr(exclusionAddr, "policy_ids"),
				resource.TestCheckNoResourceAttr(exclusionAddr, "instance_ids"),
				resource.TestCheckResourceAttrSet(exclusionAddr, "created_at"),
			),
		},
		resource.TestStep{
			Config:           cfg,
			ConfigPlanChecks: expectAction(exclusionAddr, plancheck.ResourceActionNoop),
		},
	)
	if n := len(srv.Exclusions()); n != 0 {
		t.Errorf("exclusion should be destroyed at the end of the test, %d left", n)
	}
}

func TestThreatDetectionExclusion_temporaryLifecycleAndStateToggle(t *testing.T) {
	srv := fakereco.New(t)
	const until = "2099-01-01T00:00:00Z"
	unitTest(t,
		resource.TestStep{
			Config: unitScopedExclusionConfig(srv, "Scoped exclusion", client.ExclusionStateOnPermanent, ""),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(exclusionAddr, "policy_ids.#", "1"),
				resource.TestCheckResourceAttrSet(exclusionAddr, "policy_id"),
				resource.TestCheckNoResourceAttr(exclusionAddr, "description"),
				resource.TestCheckNoResourceAttr(exclusionAddr, "valid_until"),
				func(*terraform.State) error {
					if countStateCalls(srv) != 0 {
						return fmt.Errorf("create should not call the state endpoint, requests: %v", srv.Requests())
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config:           unitScopedExclusionConfig(srv, "Scoped exclusion renamed", client.ExclusionStateOnPermanent, ""),
			ConfigPlanChecks: expectAction(exclusionAddr, plancheck.ResourceActionUpdate),
			Check: func(*terraform.State) error {
				if countStateCalls(srv) != 0 {
					return fmt.Errorf("renaming should not call the state endpoint, requests: %v", srv.Requests())
				}
				if findExclusion(srv, "Scoped exclusion renamed") == nil || len(srv.Exclusions()) != 1 {
					return fmt.Errorf("rename should update in place, API has %+v", srv.Exclusions())
				}
				return nil
			},
		},
		resource.TestStep{
			Config:           unitScopedExclusionConfig(srv, "Scoped exclusion renamed", client.ExclusionStateOnTemporary, until),
			ConfigPlanChecks: expectAction(exclusionAddr, plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(exclusionAddr, "state", client.ExclusionStateOnTemporary),
				resource.TestCheckResourceAttr(exclusionAddr, "valid_until", until),
				func(*terraform.State) error {
					got := findExclusion(srv, "Scoped exclusion renamed")
					if got == nil || got.ValidUntil == nil || *got.ValidUntil == until {
						return fmt.Errorf("API should store the temporary expiry, got %+v", got)
					}
					return nil
				},
				func(*terraform.State) error {
					if countStateCalls(srv) != 1 {
						return fmt.Errorf("turning on temporarily should call the state endpoint once, requests: %v", srv.Requests())
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config:           unitScopedExclusionConfig(srv, "Scoped exclusion renamed", client.ExclusionStateOnTemporary, until),
			ConfigPlanChecks: expectAction(exclusionAddr, plancheck.ResourceActionNoop),
		},
		resource.TestStep{
			Config:           unitScopedExclusionConfig(srv, "Scoped exclusion renamed", client.ExclusionStateOff, ""),
			ConfigPlanChecks: expectAction(exclusionAddr, plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(exclusionAddr, "state", client.ExclusionStateOff),
				resource.TestCheckNoResourceAttr(exclusionAddr, "valid_until"),
				func(*terraform.State) error {
					got := findExclusion(srv, "Scoped exclusion renamed")
					if got == nil || got.State != client.ExclusionStateOff || got.ValidUntil != nil {
						return fmt.Errorf("OFF should clear expiry, API has %+v", got)
					}
					if countStateCalls(srv) != 2 {
						return fmt.Errorf("turning off should call the state endpoint, requests: %v", srv.Requests())
					}
					return nil
				},
			),
		},
		resource.TestStep{
			ResourceName:      exclusionAddr,
			ImportState:       true,
			ImportStateVerify: true,
		},
	)
}

func TestThreatDetectionExclusion_validation(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{
			Config:      unitGlobalExclusionConfig(srv, "Missing expiry", client.ExclusionStateOnTemporary, "", ""),
			PlanOnly:    true,
			ExpectError: errorMatching(`valid_until is required when state is VIOLATION_EXCLUSION_STATE_ON_TEMPORARY`),
		},
		resource.TestStep{
			Config:      unitGlobalExclusionConfig(srv, "Bad expiry", client.ExclusionStateOnTemporary, "next tuesday", ""),
			PlanOnly:    true,
			ExpectError: errorMatching(`not a valid RFC3339 timestamp`),
		},
		resource.TestStep{
			Config:      unitGlobalExclusionConfig(srv, "Expiry while off", client.ExclusionStateOff, "2099-01-01T00:00:00Z", ""),
			PlanOnly:    true,
			ExpectError: errorMatching(`Omit valid_until unless state is VIOLATION_EXCLUSION_STATE_ON_TEMPORARY`),
		},
		resource.TestStep{
			Config:      unitGlobalExclusionConfig(srv, "Bad state", "ENABLED", "", ""),
			PlanOnly:    true,
			ExpectError: errorMatching(`state[\s\S]*must be one of`),
		},
		resource.TestStep{
			Config: srv.ProviderConfig() + `
resource "reco_threat_detection_exclusion" "test" {
  name       = "No scope"
  expression = "true"
}
`,
			PlanOnly:    true,
			ExpectError: errorMatching(`Set policy_ids, or set is_global to true`),
		},
		resource.TestStep{
			Config: srv.ProviderConfig() + `
resource "reco_threat_detection_exclusion" "test" {
  name       = "Global and policies"
  expression = "true"
  is_global  = true
  policy_ids = ["policy-1"]
}
`,
			PlanOnly:    true,
			ExpectError: errorMatching(`policy_ids must be omitted when is_global is true`),
		},
	)
}

func TestThreatDetectionExclusion_recreatedWhenDeletedOutOfBand(t *testing.T) {
	srv := fakereco.New(t)
	cfg := unitGlobalExclusionConfig(srv, "Drift exclusion", "", "", "")
	unitTest(t,
		resource.TestStep{Config: cfg},
		resource.TestStep{
			PreConfig:        func() { srv.DeleteExclusion(findExclusion(srv, "Drift exclusion").ID) },
			Config:           cfg,
			ConfigPlanChecks: expectAction(exclusionAddr, plancheck.ResourceActionCreate),
			Check: func(*terraform.State) error {
				if findExclusion(srv, "Drift exclusion") == nil {
					return fmt.Errorf("exclusion was not recreated")
				}
				return nil
			},
		},
	)
}

func TestThreatDetectionExclusion_readFailureAfterCreateKeepsID(t *testing.T) {
	srv := fakereco.New(t)
	cfg := unitGlobalExclusionConfig(srv, "Orphan guard", "", "", "")
	srv.FailWith("GET", "/exclusions/list", 400, "list unavailable")
	unitTest(t,
		resource.TestStep{
			Config:      cfg,
			ExpectError: errorMatching(`exclusion exclusion-1 was created but could not be read[\s\S]*list unavailable`),
		},
		resource.TestStep{
			PreConfig: func() {
				if n := len(srv.Exclusions()); n != 1 {
					t.Fatalf("created exclusion should still exist after the read failure, got %d", n)
				}
				srv.ClearFailures()
			},
			Config:           cfg,
			ConfigPlanChecks: expectAction(exclusionAddr, plancheck.ResourceActionDestroyBeforeCreate),
			Check: func(*terraform.State) error {
				if n := len(srv.Exclusions()); n != 1 {
					return fmt.Errorf("replacement should leave one exclusion, got %d", n)
				}
				if srv.CountRequests(http.MethodDelete, "/exclusions/exclusion-1") != 1 {
					return fmt.Errorf("saved id exclusion-1 was not deleted before the replacement create")
				}
				return nil
			},
		},
	)
}

func TestThreatDetectionExclusion_createErrorIsReported(t *testing.T) {
	srv := fakereco.New(t)
	srv.FailWith("POST", "/exclusions/create", 400, "expression must not be empty")
	unitTest(t, resource.TestStep{
		Config:      unitGlobalExclusionConfig(srv, "Create error", "", "", ""),
		ExpectError: errorMatching(`Error creating threat detection exclusion[\s\S]*expression must not be empty`),
	})
}

func TestThreatDetectionExclusion_importUnknownID(t *testing.T) {
	srv := fakereco.New(t)
	cfg := unitGlobalExclusionConfig(srv, "Import missing", "", "", "")
	unitTest(t,
		resource.TestStep{Config: cfg},
		resource.TestStep{
			Config:        cfg,
			ResourceName:  exclusionAddr,
			ImportState:   true,
			ImportStateId: "exclusion-missing",
			ExpectError:   errorMatching(`cannot find exclusion "exclusion-missing"`),
		},
	)
}

func TestThreatDetectionExclusion_descriptionRoundTrip(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{
			Config: unitGlobalExclusionConfig(srv, "Described", "", "", "suppress the break-glass account"),
			Check:  resource.TestCheckResourceAttr(exclusionAddr, "description", "suppress the break-glass account"),
		},
		resource.TestStep{
			Config:           unitGlobalExclusionConfig(srv, "Described", "", "", ""),
			ConfigPlanChecks: expectAction(exclusionAddr, plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckNoResourceAttr(exclusionAddr, "description"),
				func(*terraform.State) error {
					got := findExclusion(srv, "Described")
					if got == nil || got.Description != "" {
						return fmt.Errorf("clearing description should store an empty description, API has %+v", got)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config:           unitGlobalExclusionConfig(srv, "Described", "", "", ""),
			ConfigPlanChecks: expectAction(exclusionAddr, plancheck.ResourceActionNoop),
		},
	)
}
