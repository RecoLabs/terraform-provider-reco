//nolint:goconst // tests repeat resource addresses and API field names on purpose
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

func unitAPIKeyConfig(srv *fakereco.Server, name, role, extra string) string {
	return srv.ProviderConfig() + fmt.Sprintf(`
resource "reco_api_key" "test" {
  name = %q
  role = %q
  %s
}
`, name, role, extra)
}

func findAPIKey(srv *fakereco.Server, name string) *client.ApiKey {
	for _, k := range srv.APIKeys() {
		if k.Name == name {
			return &k
		}
	}
	return nil
}

func TestAPIKey_lifecycle(t *testing.T) {
	srv := fakereco.New(t)
	var firstID, firstSecret string
	unitTest(t,
		resource.TestStep{
			Config: unitAPIKeyConfig(srv, "key-one", "ROLE_SECURITY_VIEWER", ""),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_api_key.test", "name", "key-one"),
				resource.TestCheckResourceAttr("reco_api_key.test", "role", "ROLE_SECURITY_VIEWER"),
				resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
				resource.TestCheckResourceAttr("reco_api_key.test", "permitted_ips.#", "0"),
				resource.TestCheckNoResourceAttr("reco_api_key.test", "expiration"),
				resource.TestCheckResourceAttrSet("reco_api_key.test", "created_by"),
				resource.TestCheckResourceAttrSet("reco_api_key.test", "created_at"),
				func(s *terraform.State) error {
					rs := s.RootModule().Resources["reco_api_key.test"]
					firstID, firstSecret = rs.Primary.ID, rs.Primary.Attributes["secret"]
					if firstSecret == "" || firstSecret != "secret-for-"+firstID {
						return fmt.Errorf("secret %q was not captured from the create response (id %q)", firstSecret, firstID)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config:           unitAPIKeyConfig(srv, "key-renamed", "ROLE_ADMIN", `permitted_ips = ["10.0.0.1", "10.0.0.2"]`),
			ConfigPlanChecks: expectAction("reco_api_key.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_api_key.test", "name", "key-renamed"),
				resource.TestCheckResourceAttr("reco_api_key.test", "role", "ROLE_ADMIN"),
				resource.TestCheckResourceAttr("reco_api_key.test", "permitted_ips.#", "2"),
				func(s *terraform.State) error {
					rs := s.RootModule().Resources["reco_api_key.test"]
					if rs.Primary.ID != firstID || rs.Primary.Attributes["secret"] != firstSecret {
						return fmt.Errorf("id/secret changed on in-place update: %q/%q", rs.Primary.ID, rs.Primary.Attributes["secret"])
					}
					if k := findAPIKey(srv, "key-renamed"); k == nil || k.Role != "ROLE_ADMIN" || len(k.PermittedIps) != 2 {
						return fmt.Errorf("update not applied on the API side: %+v", k)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config: unitAPIKeyConfig(srv, "key-renamed", "ROLE_ADMIN", `permitted_ips = []`),
			Check:  resource.TestCheckResourceAttr("reco_api_key.test", "permitted_ips.#", "0"),
		},
		resource.TestStep{
			ResourceName:            "reco_api_key.test",
			ImportState:             true,
			ImportStateIdFunc:       func(*terraform.State) (string, error) { return firstID, nil },
			ImportStateVerify:       true,
			ImportStateVerifyIgnore: []string{"secret"}, // the API only returns the secret on creation
		},
	)
	if n := len(srv.APIKeys()); n != 0 {
		t.Errorf("key should be destroyed at the end of the test, %d left", n)
	}
}

func TestAPIKey_withExpiration(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t, resource.TestStep{
		Config: unitAPIKeyConfig(srv, "expiring", "ROLE_SECURITY_VIEWER", `expiration = "2031-05-06T07:08:09Z"`),
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr("reco_api_key.test", "expiration", "2031-05-06T07:08:09Z"),
			func(*terraform.State) error {
				if k := findAPIKey(srv, "expiring"); k == nil || k.Expiration == nil || *k.Expiration != "2031-05-06T07:08:09Z" {
					return fmt.Errorf("expiration not sent to the API: %+v", k)
				}
				return nil
			},
		),
	})
}

func TestAPIKey_stateLifecycle(t *testing.T) {
	srv := fakereco.New(t)
	var id, secret string
	active := unitAPIKeyConfig(srv, "lifecycle", "ROLE_SECURITY_VIEWER", "")
	retired := unitAPIKeyConfig(srv, "lifecycle", "ROLE_SECURITY_VIEWER", `state = "APPLICATION_KEY_STATE_RETIRED"`)
	explicitActive := unitAPIKeyConfig(srv, "lifecycle", "ROLE_SECURITY_VIEWER", `state = "APPLICATION_KEY_STATE_ACTIVE"`)
	unitTest(t,
		resource.TestStep{
			Config: active,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
				func(s *terraform.State) error {
					rs := s.RootModule().Resources["reco_api_key.test"]
					id, secret = rs.Primary.ID, rs.Primary.Attributes["secret"]
					if k := findAPIKey(srv, "lifecycle"); k == nil || k.State != "APPLICATION_KEY_STATE_ACTIVE" || k.ID != id {
						return fmt.Errorf("created key = %+v, state id %q", k, id)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config:           active,
			ConfigPlanChecks: expectAction("reco_api_key.test", plancheck.ResourceActionNoop),
		},
		resource.TestStep{
			Config:           explicitActive,
			ConfigPlanChecks: expectAction("reco_api_key.test", plancheck.ResourceActionNoop),
		},
		resource.TestStep{
			Config:           retired,
			ConfigPlanChecks: expectAction("reco_api_key.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_RETIRED"),
				func(s *terraform.State) error {
					rs := s.RootModule().Resources["reco_api_key.test"]
					if rs.Primary.ID != id || rs.Primary.Attributes["secret"] != secret {
						return fmt.Errorf("retire replaced the key: id %q secret %q", rs.Primary.ID, rs.Primary.Attributes["secret"])
					}
					if k := findAPIKey(srv, "lifecycle"); k == nil || k.State != "APPLICATION_KEY_STATE_RETIRED" || k.ID != id {
						return fmt.Errorf("API key was not soft-retired in place: %+v", k)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config:           explicitActive,
			ConfigPlanChecks: expectAction("reco_api_key.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
				func(s *terraform.State) error {
					rs := s.RootModule().Resources["reco_api_key.test"]
					if rs.Primary.ID != id || rs.Primary.Attributes["secret"] != secret {
						return fmt.Errorf("reactivate replaced the key: id %q secret %q", rs.Primary.ID, rs.Primary.Attributes["secret"])
					}
					if k := findAPIKey(srv, "lifecycle"); k == nil || k.State != "APPLICATION_KEY_STATE_ACTIVE" {
						return fmt.Errorf("API key was not restored: %+v", k)
					}
					return nil
				},
			),
		},
	)
}

func TestAPIKey_createRetired(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t, resource.TestStep{
		Config: unitAPIKeyConfig(srv, "born-retired", "ROLE_SECURITY_VIEWER", `state = "APPLICATION_KEY_STATE_RETIRED"`),
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_RETIRED"),
			resource.TestCheckResourceAttrSet("reco_api_key.test", "secret"),
			func(s *terraform.State) error {
				rs := s.RootModule().Resources["reco_api_key.test"]
				k := findAPIKey(srv, "born-retired")
				if k == nil || k.State != "APPLICATION_KEY_STATE_RETIRED" || k.ID != rs.Primary.ID {
					return fmt.Errorf("create did not retire the key in place: %+v", k)
				}
				if rs.Primary.Attributes["secret"] != "secret-for-"+k.ID {
					return fmt.Errorf("secret was not kept: %q", rs.Primary.Attributes["secret"])
				}
				return nil
			},
		),
	})
}

func TestAPIKey_stateDriftIsRefreshed(t *testing.T) {
	srv := fakereco.New(t)
	active := unitAPIKeyConfig(srv, "drift-state", "ROLE_SECURITY_VIEWER", `permitted_ips = []`)
	retired := unitAPIKeyConfig(srv, "drift-state", "ROLE_SECURITY_VIEWER", "permitted_ips = []\n  state = \"APPLICATION_KEY_STATE_RETIRED\"")
	var id, secret string
	unitTest(t,
		resource.TestStep{
			Config: active,
			Check: func(s *terraform.State) error {
				rs := s.RootModule().Resources["reco_api_key.test"]
				id, secret = rs.Primary.ID, rs.Primary.Attributes["secret"]
				return nil
			},
		},
		resource.TestStep{
			PreConfig: func() {
				srv.ModifyAPIKey(id, func(k *client.ApiKey) {
					k.State = "APPLICATION_KEY_STATE_RETIRED"
				})
			},
			Config:           retired,
			ConfigPlanChecks: expectAction("reco_api_key.test", plancheck.ResourceActionNoop),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_RETIRED"),
				func(s *terraform.State) error {
					rs := s.RootModule().Resources["reco_api_key.test"]
					if rs.Primary.ID != id || rs.Primary.Attributes["secret"] != secret {
						return fmt.Errorf("refresh replaced the key: id %q", rs.Primary.ID)
					}
					if k := findAPIKey(srv, "drift-state"); k == nil || k.State != "APPLICATION_KEY_STATE_RETIRED" {
						return fmt.Errorf("refresh wrote state back to the API: %+v", k)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config:             active,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		resource.TestStep{
			Config:           active,
			ConfigPlanChecks: expectAction("reco_api_key.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
				func(s *terraform.State) error {
					rs := s.RootModule().Resources["reco_api_key.test"]
					if rs.Primary.ID != id || rs.Primary.Attributes["secret"] != secret {
						return fmt.Errorf("drift repair replaced the key: id %q", rs.Primary.ID)
					}
					if k := findAPIKey(srv, "drift-state"); k == nil || k.State != "APPLICATION_KEY_STATE_ACTIVE" {
						return fmt.Errorf("plan did not restore the drifted key: %+v", k)
					}
					return nil
				},
			),
		},
	)
}

func TestAPIKey_rejectsInvalidState(t *testing.T) {
	srv := fakereco.New(t)
	var created bool
	unitTest(t,
		resource.TestStep{
			Config:      unitAPIKeyConfig(srv, "bad-state", "ROLE_SECURITY_VIEWER", `state = "APPLICATION_KEY_STATE_DELETE"`),
			ExpectError: errorMatching(`(?s)state.*must be one of`),
		},
		resource.TestStep{
			PreConfig: func() {
				created = findAPIKey(srv, "bad-state") != nil
			},
			Config: unitAPIKeyConfig(srv, "bad-state", "ROLE_SECURITY_VIEWER", ""),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
				func(*terraform.State) error {
					if created {
						return fmt.Errorf("invalid state created a key before validation")
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config:      unitAPIKeyConfig(srv, "bad-state", "ROLE_SECURITY_VIEWER", `state = "nope"`),
			ExpectError: errorMatching(`(?s)state.*must be one of`),
		},
		resource.TestStep{
			Config: unitAPIKeyConfig(srv, "bad-state", "ROLE_SECURITY_VIEWER", ""),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
				func(*terraform.State) error {
					if k := findAPIKey(srv, "bad-state"); k == nil || k.State != "APPLICATION_KEY_STATE_ACTIVE" {
						return fmt.Errorf("invalid state update changed the key: %+v", k)
					}
					return nil
				},
			),
		},
	)
}

func TestAPIKey_recreatedWhenDeletedOutOfBand(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitAPIKeyConfig(srv, "drift", "ROLE_SECURITY_VIEWER", "")},
		resource.TestStep{
			PreConfig:        func() { srv.DeleteAPIKey(findAPIKey(srv, "drift").ID) },
			Config:           unitAPIKeyConfig(srv, "drift", "ROLE_SECURITY_VIEWER", ""),
			ConfigPlanChecks: expectAction("reco_api_key.test", plancheck.ResourceActionCreate),
			Check: func(*terraform.State) error {
				if findAPIKey(srv, "drift") == nil {
					return fmt.Errorf("key was not recreated")
				}
				return nil
			},
		},
	)
}

func TestAPIKey_duplicateNameIsReported(t *testing.T) {
	srv := fakereco.New(t)
	srv.SeedAPIKey(client.ApiKey{Name: "taken", Role: "ROLE_ADMIN"})
	unitTest(t, resource.TestStep{
		Config:      unitAPIKeyConfig(srv, "taken", "ROLE_SECURITY_VIEWER", ""),
		ExpectError: errorMatching(`Error creating API key[\s\S]*already exists`),
	})
}

func TestAPIKey_importUnknownKey(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitAPIKeyConfig(srv, "k", "ROLE_SECURITY_VIEWER", "")},
		resource.TestStep{
			Config:        unitAPIKeyConfig(srv, "k", "ROLE_SECURITY_VIEWER", ""),
			ResourceName:  "reco_api_key.test",
			ImportState:   true,
			ImportStateId: "key-999",
			ExpectError:   errorMatching(`cannot find API key "key-999"`),
		},
	)
}
