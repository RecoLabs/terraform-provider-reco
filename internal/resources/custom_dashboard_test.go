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

func TestAccCustomDashboard_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	name := fmt.Sprintf("TF Acc Dashboard %d", testTimestamp())
	renamed := name + " Renamed"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		// The API cannot delete dashboards, so destroy only drops state.
		Steps: []resource.TestStep{
			{
				Config: testAccDashboardConfig(name, "DASHBOARD_VISIBILITY_PRIVATE"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_custom_dashboard.test", "name", name),
					resource.TestCheckResourceAttrSet("reco_custom_dashboard.test", "id"),
				),
			},
			{
				Config: testAccDashboardConfig(renamed, "DASHBOARD_VISIBILITY_SHARED_READ"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_custom_dashboard.test", "name", renamed),
					resource.TestCheckResourceAttr("reco_custom_dashboard.test", "visibility", "DASHBOARD_VISIBILITY_SHARED_READ"),
					checkDashboardExists("reco_custom_dashboard.test"),
				),
			},
		},
	})
}

func TestAccCustomDashboard_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	name := fmt.Sprintf("TF Acc Dashboard Import %d", testTimestamp())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{Config: testAccDashboardConfig(name, "DASHBOARD_VISIBILITY_PRIVATE")},
			{
				ResourceName:      "reco_custom_dashboard.test",
				ImportState:       true,
				ImportStateVerify: true,
				// The server may re-serialize spec; compare semantically in Read instead.
				ImportStateVerifyIgnore: []string{"spec"},
			},
		},
	})
}

func testAccDashboardConfig(name, visibility string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_custom_dashboard" "test" {
  name       = %q
  visibility = %q
  spec       = jsonencode({ sections = [] })
}
`, name, visibility)
}

func checkDashboardExists(addr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("resource %s not found in state", addr)
		}
		c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
		_, err := c.GetDashboardByID(context.Background(), rs.Primary.ID)
		return err
	}
}
