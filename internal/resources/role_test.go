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

func TestAccRecoRole_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	name := fmt.Sprintf("tf-acc-role-%d", testTimestamp())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkRoleDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: testAccRoleConfig(name, "Acceptance test role"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_role.test", "name", name),
					resource.TestCheckResourceAttr("reco_role.test", "description", "Acceptance test role"),
				),
			},
		},
	})
}

func TestAccRecoRole_update(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	name := fmt.Sprintf("tf-acc-role-upd-%d", testTimestamp())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkRoleDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: testAccRoleConfig(name, "Original description"),
				Check:  resource.TestCheckResourceAttr("reco_role.test", "description", "Original description"),
			},
			{
				Config: testAccRoleConfig(name, "Updated description"),
				Check:  resource.TestCheckResourceAttr("reco_role.test", "description", "Updated description"),
			},
		},
	})
}

func TestAccRecoRole_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	name := fmt.Sprintf("tf-acc-role-imp-%d", testTimestamp())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkRoleDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: testAccRoleConfig(name, "Import test role"),
			},
			{
				ResourceName:      "reco_role.test",
				ImportState:       true,
				ImportStateId:     name,
				ImportStateVerify: true,
			},
		},
	})
}

func checkRoleDestroyed(name string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
		_, err := c.GetRoleByName(context.Background(), name)
		if client.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("role %q still exists after destroy", name)
	}
}

func testAccRoleConfig(name, description string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_role" "test" {
  name        = %q
  description = %q
  permissions = [%q]
}
`, name, description, testAccPermission())
}
