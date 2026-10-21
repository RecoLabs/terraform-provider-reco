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

func unitRoleConfig(srv *fakereco.Server, name, description string) string {
	return srv.ProviderConfig() + fmt.Sprintf(`
resource "reco_role" "test" {
  name        = %q
  description = %q
  permissions = ["PERM_ALERTS_READ", "PERM_USERS_READ"]
}
`, name, description)
}

func findRole(srv *fakereco.Server, name string) *client.RecoRole {
	for _, r := range srv.Roles() {
		if r.Name == name {
			return &r
		}
	}
	return nil
}

func TestRole_lifecycle(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{
			Config: unitRoleConfig(srv, "r1", "first"),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_role.test", "id", "r1"),
				resource.TestCheckResourceAttr("reco_role.test", "name", "r1"),
				resource.TestCheckResourceAttr("reco_role.test", "description", "first"),
				resource.TestCheckResourceAttr("reco_role.test", "permissions.#", "2"),
				resource.TestCheckResourceAttr("reco_role.test", "permissions.0", "PERM_ALERTS_READ"),
				resource.TestCheckResourceAttr("reco_role.test", "type", "USER_ROLE_TYPE_CUSTOM"),
				resource.TestCheckResourceAttrSet("reco_role.test", "created_at"),
				func(_ *terraform.State) error {
					if r := findRole(srv, "r1"); r == nil || len(r.Permissions) != 2 {
						return fmt.Errorf("role not stored correctly on the API side: %+v", r)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config:           unitRoleConfig(srv, "r1", "second"),
			ConfigPlanChecks: expectAction("reco_role.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_role.test", "description", "second"),
				func(_ *terraform.State) error {
					if r := findRole(srv, "r1"); r == nil || r.Description != "second" {
						return fmt.Errorf("update was not applied on the API side: %+v", r)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			ResourceName:      "reco_role.test",
			ImportState:       true,
			ImportStateId:     "r1",
			ImportStateVerify: true,
		},
	)
	if r := findRole(srv, "r1"); r != nil {
		t.Errorf("role should be destroyed at the end of the test, still have %+v", r)
	}
}

func TestRole_renameForcesReplacement(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitRoleConfig(srv, "old-name", "d")},
		resource.TestStep{
			Config:           unitRoleConfig(srv, "new-name", "d"),
			ConfigPlanChecks: expectAction("reco_role.test", plancheck.ResourceActionDestroyBeforeCreate),
			Check: func(_ *terraform.State) error {
				if findRole(srv, "old-name") != nil || findRole(srv, "new-name") == nil {
					return fmt.Errorf("expected old-name to be replaced by new-name, got %+v", srv.Roles())
				}
				return nil
			},
		},
	)
}

func TestRole_withResourcePermissions(t *testing.T) {
	srv := fakereco.New(t)
	cfg := srv.ProviderConfig() + `
resource "reco_role" "test" {
  name = "scoped"
  resources = [
    { resource_name = "Finance", resource = "segment-1", permission = "PERM_ALERTS_READ" },
    { resource_name = "HR", resource = "segment-2", permission = "PERM_ALERTS_WRITE" },
  ]
}
`
	unitTest(t, resource.TestStep{
		Config: cfg,
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr("reco_role.test", "resources.#", "2"),
			resource.TestCheckResourceAttr("reco_role.test", "resources.1.resource_name", "HR"),
			resource.TestCheckResourceAttr("reco_role.test", "resources.1.resource", "segment-2"),
			resource.TestCheckResourceAttr("reco_role.test", "resources.1.permission", "PERM_ALERTS_WRITE"),
			resource.TestCheckResourceAttr("reco_role.test", "permissions.#", "0"),
		),
	})
}

func TestRole_recreatedWhenDeletedOutOfBand(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitRoleConfig(srv, "drift", "d")},
		resource.TestStep{
			PreConfig:        func() { srv.DeleteRole("drift") },
			Config:           unitRoleConfig(srv, "drift", "d"),
			ConfigPlanChecks: expectAction("reco_role.test", plancheck.ResourceActionCreate),
			Check: func(_ *terraform.State) error {
				if findRole(srv, "drift") == nil {
					return fmt.Errorf("role was not recreated")
				}
				return nil
			},
		},
	)
}

func TestRole_createErrorIsReported(t *testing.T) {
	srv := fakereco.New(t)
	srv.FailWith("POST", "/roles/create", 400, "unknown permission PERM_ALERTS_READ")
	unitTest(t, resource.TestStep{
		Config:      unitRoleConfig(srv, "bad", "d"),
		ExpectError: errorMatching(`Error creating role[\s\S]*unknown permission`),
	})
}

func TestRole_duplicateNameIsReported(t *testing.T) {
	srv := fakereco.New(t)
	srv.SeedRole(client.RecoRole{Name: "taken"})
	unitTest(t, resource.TestStep{
		Config:      unitRoleConfig(srv, "taken", "d"),
		ExpectError: errorMatching(`Error creating role[\s\S]*already exists`),
	})
}

func TestRole_importUnknownRole(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitRoleConfig(srv, "exists", "d")},
		resource.TestStep{
			Config:        unitRoleConfig(srv, "exists", "d"),
			ResourceName:  "reco_role.test",
			ImportState:   true,
			ImportStateId: "does-not-exist",
			ExpectError:   errorMatching(`cannot find role "does-not-exist"`),
		},
	)
}

func TestRole_readErrorIsReported(t *testing.T) {
	srv := fakereco.New(t)
	unitTest(t,
		resource.TestStep{Config: unitRoleConfig(srv, "r", "d")},
		resource.TestStep{
			PreConfig:   func() { srv.FailWith("GET", "/roles/list", 403, "forbidden") },
			Config:      unitRoleConfig(srv, "r", "d"),
			ExpectError: errorMatching(`Error reading role[\s\S]*HTTP 403`),
		},
		// Let the final destroy succeed.
		resource.TestStep{
			PreConfig: srv.ClearFailures,
			Config:    unitRoleConfig(srv, "r", "d"),
		},
	)
}
