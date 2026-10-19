package resources_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

func unitUserConfig(srv *fakereco.Server, email, name, extra string) string {
	return unitProviderConfig(srv) + fmt.Sprintf(`
resource "reco_user" "test" {
  email_address = %q
  name          = %q
  user_roles    = ["ROLE_SECURITY_VIEWER"]
  %s
}
`, email, name, extra)
}

func findUser(srv *fakereco.Server, email string) *client.RecoUser {
	list := srv.Users()
	for i := range list {
		if list[i].EmailAddress == email {
			return &list[i]
		}
	}
	return nil
}

func userIDOf(srv *fakereco.Server, email string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		u := findUser(srv, email)
		if u == nil {
			return fmt.Errorf("no user with email %q on the API side", email)
		}
		return resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr("reco_user.test", "user_id", u.UserID),
			resource.TestCheckResourceAttr("reco_user.test", "id", u.UserID),
		)(s)
	}
}

func TestUser_lifecycle(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{
			Config: unitUserConfig(srv, "a@example.com", "Alice", ""),
			Check: resource.ComposeTestCheckFunc(
				userIDOf(srv, "a@example.com"),
				resource.TestCheckResourceAttr("reco_user.test", "email_address", "a@example.com"),
				resource.TestCheckResourceAttr("reco_user.test", "name", "Alice"),
				resource.TestCheckResourceAttr("reco_user.test", "user_roles.#", "1"),
				resource.TestCheckResourceAttr("reco_user.test", "user_roles.0", "ROLE_SECURITY_VIEWER"),
				resource.TestCheckResourceAttr("reco_user.test", "segments.#", "0"),
				resource.TestCheckResourceAttr("reco_user.test", "is_bypass_sso_enforcement", "false"),
				resource.TestCheckResourceAttr("reco_user.test", "enablement_status", "enabled"),
				resource.TestCheckResourceAttr("reco_user.test", "auth_methods.#", "1"),
				resource.TestCheckResourceAttrSet("reco_user.test", "creation_time"),
				resource.TestCheckNoResourceAttr("reco_user.test", "expiration"),
			),
		},
		resource.TestStep{
			Config:           unitUserConfig(srv, "a@example.com", "Alice Renamed", ""),
			ConfigPlanChecks: expectAction("reco_user.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				userIDOf(srv, "a@example.com"),
				resource.TestCheckResourceAttr("reco_user.test", "name", "Alice Renamed"),
			),
		},
		resource.TestStep{
			ResourceName:      "reco_user.test",
			ImportState:       true,
			ImportStateId:     "a@example.com",
			ImportStateVerify: true,
		},
		resource.TestStep{
			ResourceName: "reco_user.test",
			ImportState:  true,
			ImportStateIdFunc: func(*terraform.State) (string, error) {
				return findUser(srv, "a@example.com").UserID, nil
			},
			ImportStateVerify: true,
		},
	)
	if n := len(srv.Users()); n != 0 {
		t.Errorf("user should be destroyed at the end of the test, %d left", n)
	}
}

func TestUser_changingEmailUpdatesInPlace(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitUserConfig(srv, "old@example.com", "Bob", "")},
		resource.TestStep{
			Config:           unitUserConfig(srv, "new@example.com", "Bob", ""),
			ConfigPlanChecks: expectAction("reco_user.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				userIDOf(srv, "new@example.com"),
				resource.TestCheckResourceAttr("reco_user.test", "email_address", "new@example.com"),
				func(*terraform.State) error {
					if len(srv.Users()) != 1 {
						return fmt.Errorf("want exactly one user after an email change, got %+v", srv.Users())
					}
					return nil
				},
			),
		},
	)
}

func TestUser_enablementStatusAndBypassAndSegments(t *testing.T) {
	srv := fakereco.New(t)
	extra := `
  enablement_status         = "disabled"
  is_bypass_sso_enforcement = true
  segments                  = ["finance", "hr"]`
	unitTest(t,
		resource.TestStep{Config: unitUserConfig(srv, "s@example.com", "S", "")},
		resource.TestStep{
			Config: unitUserConfig(srv, "s@example.com", "S", extra),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_user.test", "enablement_status", "disabled"),
				resource.TestCheckResourceAttr("reco_user.test", "is_bypass_sso_enforcement", "true"),
				resource.TestCheckResourceAttr("reco_user.test", "segments.#", "2"),
				func(*terraform.State) error {
					u := findUser(srv, "s@example.com")
					if u.EnablementStatus != "disabled" || !u.IsBypassSSOEnforcement || len(u.Segments) != 2 {
						return fmt.Errorf("API-side user does not match: %+v", u)
					}
					return nil
				},
			),
		},
	)
}

func TestUser_expiration(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{
			Config: unitUserConfig(srv, "e@example.com", "E", `expiration = "2030-01-01T00:00:00Z"`),
			Check:  resource.TestCheckResourceAttr("reco_user.test", "expiration", "2030-01-01T00:00:00Z"),
		},
		resource.TestStep{
			Config: unitUserConfig(srv, "e@example.com", "E", ""),
			Check:  resource.TestCheckNoResourceAttr("reco_user.test", "expiration"),
		},
	)
}

func TestUser_emptyRolesRejectedAtPlanTime(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t, resource.TestStep{
		Config: unitProviderConfig(srv) + `
resource "reco_user" "test" {
  email_address = "x@example.com"
  name          = "X"
  user_roles    = []
}
`,
		PlanOnly:    true,
		ExpectError: errorMatching(`list must contain at least 1 elements`),
	})
	for _, r := range srv.Requests() {
		if strings.HasPrefix(r, "POST ") {
			t.Errorf("validation should fail before any create call, saw %q", r)
		}
	}
}

func TestUser_recreatedWhenDeletedOutOfBand(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitUserConfig(srv, "d@example.com", "D", "")},
		resource.TestStep{
			PreConfig:        func() { srv.DeleteUser(findUser(srv, "d@example.com").UserID) },
			Config:           unitUserConfig(srv, "d@example.com", "D", ""),
			ConfigPlanChecks: expectAction("reco_user.test", plancheck.ResourceActionCreate),
			Check:            userIDOf(srv, "d@example.com"),
		},
	)
}

func TestUser_duplicateEmailIsReported(t *testing.T) {
	srv := fakereco.New(t)
	srv.SeedUser(client.RecoUser{EmailAddress: "dup@example.com", Name: "Existing"})
	unitTest(t, resource.TestStep{
		Config:      unitUserConfig(srv, "dup@example.com", "New", ""),
		ExpectError: errorMatching(`Error creating user[\s\S]*already exists`),
	})
}

func TestUser_importUnknownUser(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitUserConfig(srv, "i@example.com", "I", "")},
		resource.TestStep{
			Config:        unitUserConfig(srv, "i@example.com", "I", ""),
			ResourceName:  "reco_user.test",
			ImportState:   true,
			ImportStateId: "nobody@example.com",
			ExpectError:   errorMatching(`cannot find user "nobody@example.com"`),
		},
		resource.TestStep{
			Config:        unitUserConfig(srv, "i@example.com", "I", ""),
			ResourceName:  "reco_user.test",
			ImportState:   true,
			ImportStateId: "no-such-id",
			ExpectError:   errorMatching(`cannot find user "no-such-id"`),
		},
	)
}
