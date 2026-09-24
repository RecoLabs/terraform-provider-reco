package datasources_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPostureChecks_read(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheckDS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactoriesDS(),
		Steps: []resource.TestStep{
			{
				Config: `provider "reco" {}
data "reco_posture_checks" "all" {}`,
				Check: resource.TestCheckResourceAttrSet("data.reco_posture_checks.all", "posture_checks.#"),
			},
		},
	})
}
