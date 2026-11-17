//nolint:goconst // tests repeat resource addresses and API field names on purpose
package resources_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/acctestutil"
	"github.com/recolabs/terraform-provider-reco/internal/client"
)

func TestAccApiKey_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	ts := acctestutil.Suffix()
	name := fmt.Sprintf("tf-acc-key-%s", ts)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkApiKeyDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: testAccApiKeyConfig(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_api_key.test", "name", name),
					resource.TestCheckResourceAttr("reco_api_key.test", "role", testAccViewerRole),
					resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
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

func TestAccApiKey_state(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	name := fmt.Sprintf("tf-acc-key-state-%s", acctestutil.Suffix())
	var id, secret string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkApiKeyDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: testAccApiKeyConfig(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
					resource.TestCheckResourceAttrSet("reco_api_key.test", "secret"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["reco_api_key.test"]
						id, secret = rs.Primary.ID, rs.Primary.Attributes["secret"]
						if id == "" || secret == "" {
							return fmt.Errorf("missing id or secret")
						}
						return nil
					},
				),
			},
			{
				Config: testAccApiKeyConfig(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["reco_api_key.test"]
						if rs.Primary.ID != id || rs.Primary.Attributes["secret"] != secret {
							return fmt.Errorf("omitted state changed the key")
						}
						return nil
					},
				),
			},
			{
				Config: testAccApiKeyConfigWithState(name, "APPLICATION_KEY_STATE_ACTIVE"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("reco_api_key.test", "state", "APPLICATION_KEY_STATE_ACTIVE"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["reco_api_key.test"]
						if rs.Primary.ID != id || rs.Primary.Attributes["secret"] != secret {
							return fmt.Errorf("explicit active state recreated the key")
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccApiKey_invalidState(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{{
			Config:      testAccApiKeyConfigWithState("tf-acc-key-bad-state", "APPLICATION_KEY_STATE_DELETE"),
			ExpectError: regexp.MustCompile(`(?s)state.*must be one of`),
		}},
	})
}

func testAccApiKeyConfigWithState(name, state string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "reco_api_key" "test" {
  name  = %q
  role  = %q
  state = %q
}
`, name, testAccViewerRole, state)
}

func TestAccApiKey_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheck(t)

	name := fmt.Sprintf("tf-acc-key-imp-%s", acctestutil.Suffix())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkApiKeyDestroyed(name),
		Steps: []resource.TestStep{
			{Config: testAccApiKeyConfig(name)},
			{
				ResourceName:            "reco_api_key.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secret"}, // the API only returns the secret on creation
			},
		},
	})
}
