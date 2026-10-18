package resources_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

// unitProviderConfig points the provider at the fake API. Attributes take precedence over the
// RECO_* environment variables, so unit tests never reach a real tenant even if those are set.
func unitProviderConfig(srv *fakereco.Server) string {
	return fmt.Sprintf(`
provider "reco" {
  api_key  = %q
  base_url = %q
}
`, fakereco.APIKey, srv.URL)
}

// unitTest runs a test case against the fake API without requiring TF_ACC.
func unitTest(t *testing.T, steps ...resource.TestStep) {
	t.Helper()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps:                    steps,
	})
}

func expectAction(addr string, action plancheck.ResourceActionType) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, action)},
	}
}

func errorMatching(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
