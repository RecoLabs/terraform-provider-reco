package datasources_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIntegrations_read(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheckDS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactoriesDS(),
		Steps: []resource.TestStep{
			{
				Config: `provider "reco" {}
data "reco_integrations" "all" {}`,
				Check: resource.TestCheckResourceAttrSet("data.reco_integrations.all", "integrations.#"),
			},
		},
	})
}

func TestAccIntegrations_pagination(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheckDS(t)
	t.Setenv("RECO_TEST_PAGE_SIZE", "5")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactoriesDS(),
		Steps: []resource.TestStep{
			{
				Config: `provider "reco" {}
data "reco_integrations" "all" {}`,
				Check: resource.TestCheckResourceAttrWith(
					"data.reco_integrations.all", "integrations.#",
					func(v string) error {
						n, err := strconv.Atoi(v)
						if err != nil {
							return fmt.Errorf("unexpected non-integer count %q", v)
						}
						if n <= 5 {
							return fmt.Errorf("expected more than one page of integrations, got %d", n)
						}
						return nil
					},
				),
			},
		},
	})
}
