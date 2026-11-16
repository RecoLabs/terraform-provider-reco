package resources_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

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
