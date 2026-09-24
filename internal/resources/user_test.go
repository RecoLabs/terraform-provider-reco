package resources_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/provider"
)

func providerFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"reco": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("RECO_API_KEY") == "" {
		t.Fatal("RECO_API_KEY must be set for acceptance tests")
	}
	if os.Getenv("RECO_BASE_URL") == "" {
		t.Fatal("RECO_BASE_URL must be set for acceptance tests")
	}
}

func testAccPermission() string {
	if p := os.Getenv("RECO_TEST_PERMISSION"); p != "" {
		return p
	}
	return "PERM_ALERTS_READ"
}

func testAccProviderConfig() string {
	return `
provider "reco" {}
`
}

func TestAccRecoUser_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := testTimestamp()
	email := fmt.Sprintf("tf-acc-test-%d@reco-test.example.com", ts)
	roleName := fmt.Sprintf("tf-acc-user-role-%d", ts)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkUserDestroyed(email),
			checkRoleDestroyed(roleName),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig(email, "Test User", roleName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_user.test", "email_address", email),
					resource.TestCheckResourceAttr("reco_user.test", "name", "Test User"),
					resource.TestCheckResourceAttrSet("reco_user.test", "user_id"),
				),
			},
		},
	})
}

func TestAccRecoUser_update(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := testTimestamp()
	email := fmt.Sprintf("tf-acc-update-%d@reco-test.example.com", ts)
	roleName := fmt.Sprintf("tf-acc-user-role-%d", ts)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkUserDestroyed(email),
			checkRoleDestroyed(roleName),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig(email, "Original Name", roleName),
				Check:  resource.TestCheckResourceAttr("reco_user.test", "name", "Original Name"),
			},
			{
				Config: testAccUserConfig(email, "Updated Name", roleName),
				Check:  resource.TestCheckResourceAttr("reco_user.test", "name", "Updated Name"),
			},
		},
	})
}

func TestAccRecoUser_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := testTimestamp()
	email := fmt.Sprintf("tf-acc-import-%d@reco-test.example.com", ts)
	roleName := fmt.Sprintf("tf-acc-user-role-%d", ts)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkUserDestroyed(email),
			checkRoleDestroyed(roleName),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig(email, "Import Test", roleName),
			},
			{
				ResourceName:      "reco_user.test",
				ImportState:       true,
				ImportStateId:     email,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccRecoUser_empty_roles_rejected(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig() + `
resource "reco_user" "test" {
  email_address = "tf-acc-empty-roles@reco-test.example.com"
  name          = "Empty Roles Test"
  user_roles    = []
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`list must contain at least 1 elements`),
			},
		},
	})
}

func TestAccRecoUser_import_by_id(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := testTimestamp()
	email := fmt.Sprintf("tf-acc-imp-id-%d@reco-test.example.com", ts)
	roleName := fmt.Sprintf("tf-acc-user-role-%d", ts)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkUserDestroyed(email),
			checkRoleDestroyed(roleName),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig(email, "Import By ID Test", roleName),
			},
			{
				ResourceName: "reco_user.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["reco_user.test"]
					if !ok {
						return "", fmt.Errorf("reco_user.test not in state")
					}
					return rs.Primary.Attributes["user_id"], nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

func testAccUserConfig(email, name, roleName string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_role" "acc_role" {
  name        = %q
  description = "Role for user acceptance testing"
  permissions = [%q]
}

resource "reco_user" "test" {
  email_address = %q
  name          = %q
  user_roles    = [reco_role.acc_role.name]
  depends_on    = [reco_role.acc_role]
}
`, roleName, testAccPermission(), email, name)
}

func checkUserDestroyed(email string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
		_, err := c.GetUserByEmail(context.Background(), email)
		if client.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("user %q still exists after destroy", email)
	}
}

func testTimestamp() int64 {
	return time.Now().Unix()
}
