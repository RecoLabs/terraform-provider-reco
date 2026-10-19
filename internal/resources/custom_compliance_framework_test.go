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

func TestAccCustomComplianceFramework_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	title := fmt.Sprintf("TF Acc Framework %d", testTimestamp())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkComplianceFrameworkDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccComplianceFrameworkConfig(title, "Initial description", 1),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "title", title),
					resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "controls.#", "1"),
					resource.TestCheckResourceAttrSet("reco_custom_compliance_framework.test", "id"),
				),
			},
			{
				Config: testAccComplianceFrameworkConfig(title, "Updated description", 2),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "controls.#", "2"),
				),
			},
		},
	})
}

func TestAccCustomComplianceFramework_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	title := fmt.Sprintf("TF Acc Framework Import %d", testTimestamp())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkComplianceFrameworkDestroyed,
		Steps: []resource.TestStep{
			{Config: testAccComplianceFrameworkConfig(title, "Import test", 1)},
			{
				ResourceName:            "reco_custom_compliance_framework.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"controls"},
			},
		},
	})
}

func testAccComplianceFrameworkConfig(title, description string, controlCount int) string {
	cfg := testAccProviderConfig() + fmt.Sprintf(`
resource "reco_custom_compliance_framework" "test" {
  title       = %q
  description = %q
  version     = "1.0"
  type        = "custom"

  controls = [
`, title, description)
	for i := 1; i <= controlCount; i++ {
		cfg += fmt.Sprintf(`    {
      control_id = "C-%d"
      title      = "Control %d"
    },
`, i, i)
	}
	return cfg + "  ]\n}\n"
}

func checkComplianceFrameworkDestroyed(s *terraform.State) error {
	c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "reco_custom_compliance_framework" {
			continue
		}
		_, err := c.GetComplianceFrameworkByID(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("compliance framework %q still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
