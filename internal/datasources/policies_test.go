package datasources_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/recolabs/terraform-provider-reco/internal/provider"
)

func providerFactoriesDS() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"reco": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func testAccPreCheckDS(t *testing.T) {
	t.Helper()
	if os.Getenv("RECO_API_KEY") == "" {
		t.Fatal("RECO_API_KEY must be set for acceptance tests")
	}
	if os.Getenv("RECO_BASE_URL") == "" {
		t.Fatal("RECO_BASE_URL must be set for acceptance tests")
	}
}

func TestAccPolicies_read(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheckDS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactoriesDS(),
		Steps: []resource.TestStep{
			{
				Config: "provider \"reco\" {}\ndata \"reco_policies\" \"all\" {}",
				Check:  resource.TestCheckResourceAttrSet("data.reco_policies.all", "policies.#"),
			},
		},
	})
}
