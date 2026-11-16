package datasources_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/recolabs/terraform-provider-reco/internal/client"
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
	const pageSize = 5
	// Integrations can't be created through the API, so this depends on the tenant having
	// more than one page of them; skip instead of failing on a small tenant.
	c := client.New(os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY"), "test")
	existing, err := c.ListAllIntegrations(t.Context())
	if err != nil {
		t.Fatalf("listing integrations: %v", err)
	}
	if len(existing) <= pageSize {
		t.Skipf("tenant has %d integrations; need more than %d to exercise pagination", len(existing), pageSize)
	}
	t.Setenv("RECO_TEST_PAGE_SIZE", strconv.Itoa(pageSize))

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
						if n != len(existing) {
							return fmt.Errorf("expected all %d integrations across pages, got %d", len(existing), n)
						}
						return nil
					},
				),
			},
		},
	})
}
