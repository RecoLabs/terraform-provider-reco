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

func TestAccApiKey_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := testTimestamp()
	name := fmt.Sprintf("tf-acc-key-%d", ts)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkApiKeyDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: testAccApiKeyConfig(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_api_key.test", "name", name),
					resource.TestCheckResourceAttr("reco_api_key.test", "role", testAccViewerRole),
					resource.TestCheckResourceAttrSet("reco_api_key.test", "id"),
					resource.TestCheckResourceAttrSet("reco_api_key.test", "secret"),
				),
			},
			{
				Config: testAccApiKeyConfig(name + "-renamed"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_api_key.test", "name", name+"-renamed"),
					resource.TestCheckResourceAttrSet("reco_api_key.test", "secret"),
				),
			},
		},
	})
}

const testAccViewerRole = "ROLE_SECURITY_VIEWER"

func testAccApiKeyConfig(name string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_api_key" "test" {
  name = %q
  role = %q
}
`, name, testAccViewerRole)
}

func checkApiKeyDestroyed(name string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
		keys, err := c.ListAllApiKeys(context.Background())
		if err != nil {
			return err
		}
		for _, k := range keys {
			if k.Name == name {
				return fmt.Errorf("api key %q still exists after destroy", name)
			}
		}
		return nil
	}
}
