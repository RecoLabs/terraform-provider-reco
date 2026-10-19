package datasources_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccPrefix marks every object the acceptance tests create; the sweepers in internal/resources
// delete only objects carrying it.
const testAccPrefix = "tf-acc-"

func testAccSuffix() string {
	return acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
}

func testAccPermission() string {
	if p := os.Getenv("RECO_TEST_PERMISSION"); p != "" {
		return p
	}
	return "PERM_ALERTS_READ"
}

// testCheckListContains passes when some element of the list attribute has attr == want.
func testCheckListContains(addr, listAttr, attr, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s not found in state", addr)
		}
		n, err := strconv.Atoi(rs.Primary.Attributes[listAttr+".#"])
		if err != nil {
			return fmt.Errorf("%s.%s.#: %w", addr, listAttr, err)
		}
		for i := range n {
			if rs.Primary.Attributes[fmt.Sprintf("%s.%d.%s", listAttr, i, attr)] == want {
				return nil
			}
		}
		return fmt.Errorf("%s.%s has no element with %s = %q (%d elements)", addr, listAttr, attr, want, n)
	}
}

type createdCase struct {
	name     string
	dsType   string
	listAttr string
	nameAttr string
	dep      string                   // address of the resource the data source must be read after
	object   func(name string) string // HCL creating the object; name is the identifying value
	ident    func(suffix string) string
}

func createdCases() []createdCase {
	return []createdCase{
		{
			name: "roles", dep: "reco_role.test", dsType: "reco_roles", listAttr: "roles", nameAttr: "name",
			ident: func(s string) string { return testAccPrefix + "ds-role-" + s },
			object: func(name string) string {
				return fmt.Sprintf(`resource "reco_role" "test" {
  name        = %q
  description = "data source acceptance test"
  permissions = [%q]
}`, name, testAccPermission())
			},
		},
		{
			name: "api_keys", dep: "reco_api_key.test", dsType: "reco_api_keys", listAttr: "api_keys", nameAttr: "name",
			ident: func(s string) string { return testAccPrefix + "ds-key-" + s },
			object: func(name string) string {
				return fmt.Sprintf(`resource "reco_api_key" "test" {
  name = %q
  role = "ROLE_SECURITY_VIEWER"
}`, name)
			},
		},
		{
			name: "users", dep: "reco_user.test", dsType: "reco_users", listAttr: "users", nameAttr: "email_address",
			ident: func(s string) string { return testAccPrefix + "ds-user-" + s + "@reco-test.example.com" },
			object: func(email string) string {
				return fmt.Sprintf(`resource "reco_role" "acc_role" {
  name        = %q
  permissions = [%q]
}

resource "reco_user" "test" {
  email_address = %q
  name          = "Data source test"
  user_roles    = [reco_role.acc_role.name]
}`, testAccPrefix+"ds-user-role-"+testAccSuffix(), testAccPermission(), email)
			},
		},
		{
			name: "posture_checks", dep: "reco_posture_check.test", dsType: "reco_posture_checks", listAttr: "posture_checks", nameAttr: "name",
			ident: func(s string) string { return testAccPrefix + "ds-posture-" + s },
			object: func(title string) string {
				return fmt.Sprintf(`resource "reco_posture_check" "test" {
  title                  = %q
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "LOW"
  violation_risk_type    = "RISK_TYPE_USER"
  default_status         = "POLICY_STATUS_OFF"
  conditions_jsonata     = "true"
  posture_unique_jsonata = "$.id"
  posture_value_jsonata  = "$.value"
}`, title)
			},
		},
		{
			name: "policies", dep: "reco_threat_detection_policy.test", dsType: "reco_policies", listAttr: "policies", nameAttr: "name",
			ident: func(s string) string { return testAccPrefix + "ds-policy-" + s },
			object: func(title string) string {
				return fmt.Sprintf(`resource "reco_threat_detection_policy" "test" {
  title                = %q
  data_source          = "GSUITE_ADMIN_AUDIT_LOG_API"
  violation_risk_level = "LOW"
  violation_risk_type  = "RISK_TYPE_USER"
  default_status       = "POLICY_STATUS_OFF"
  conditions_jsonata   = "true"
  description_template = "\"Test alert\""
}`, title)
			},
		},
	}
}

// TestAccDataSources_listCreatedObjects creates a real object through each resource and checks the
// matching data source returns it, so the data sources are verified end to end rather than only
// "returns a list".
func TestAccDataSources_listCreatedObjects(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	testAccPreCheckDS(t)

	for _, tc := range createdCases() {
		t.Run(tc.name, func(t *testing.T) {
			ident := tc.ident(testAccSuffix())
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: providerFactoriesDS(),
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`provider "reco" {}

%s

data %q "all" {
  depends_on = [%s]
}
`, tc.object(ident), tc.dsType, tc.dep),
					Check: testCheckListContains("data."+tc.dsType+".all", tc.listAttr, tc.nameAttr, ident),
				}},
			})
		})
	}
}
