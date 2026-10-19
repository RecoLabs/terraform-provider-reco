package datasources_test

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

// dataSourceCase describes one list data source: how to seed n items on the fake API, what the
// first item should look like, and what to expect around it.
type dataSourceCase struct {
	typeName   string // e.g. reco_users
	listAttr   string // e.g. users
	listPath   string // API path (without prefix) the data source reads
	errorTitle string // diagnostic summary when listing fails
	baseCount  int    // items present on a fresh fake API
	paginated  bool   // whether the client pages through this list
	seed       func(srv *fakereco.Server, n int)
	firstItem  map[string]string // attribute (relative to <listAttr>.<index>) -> value
	firstIndex int               // index of the first seeded item in the list
}

func dataSourceCases() []dataSourceCase {
	return []dataSourceCase{
		{
			typeName: "reco_users", listAttr: "users", listPath: "/reco-users/list", errorTitle: "Error listing users", paginated: true,
			seed: func(srv *fakereco.Server, n int) {
				for i := range n {
					srv.SeedUser(client.RecoUser{
						EmailAddress: fmt.Sprintf("u%d@example.com", i), Name: fmt.Sprintf("User %d", i),
						UserRoles: []string{"ROLE_ADMIN"}, Segments: []string{"finance"}, IsBypassSSOEnforcement: true,
					})
				}
			},
			firstItem: map[string]string{
				"user_id": "user-1", "email_address": "u0@example.com", "name": "User 0", "user_roles.0": "ROLE_ADMIN",
				"segments.0": "finance", "enablement_status": "enabled", "is_bypass_sso_enforcement": "true", "auth_methods.0": "password",
			},
		},
		{
			typeName: "reco_roles", listAttr: "roles", listPath: "/roles/list", errorTitle: "Error listing roles", baseCount: 2, firstIndex: 2,
			seed: func(srv *fakereco.Server, n int) {
				for i := range n {
					srv.SeedRole(client.RecoRole{
						Name: fmt.Sprintf("custom-%d", i), Description: "custom role", Permissions: []string{"PERM_ALERTS_READ"},
						Resources: []client.ResourcePermission{{ResourceName: "Finance", Resource: "seg-1", Permission: "PERM_ALERTS_READ"}},
					})
				}
			},
			firstItem: map[string]string{
				"name": "custom-0", "description": "custom role", "type": "USER_ROLE_TYPE_CUSTOM", "permissions.0": "PERM_ALERTS_READ",
				"resources.0.resource_name": "Finance", "resources.0.resource": "seg-1", "resources.0.permission": "PERM_ALERTS_READ",
			},
		},
		{
			typeName: "reco_api_keys", listAttr: "api_keys", listPath: "/api-keys/list", errorTitle: "Error listing API keys",
			seed: func(srv *fakereco.Server, n int) {
				for i := range n {
					srv.SeedAPIKey(client.ApiKey{
						Name: fmt.Sprintf("key-%d", i), Role: "ROLE_ADMIN", CreatedBy: "admin@example.com",
						State: "APPLICATION_KEY_STATE_ACTIVE", PermittedIps: []string{"10.0.0.1"},
					})
				}
			},
			firstItem: map[string]string{
				"id": "key-1", "name": "key-0", "role": "ROLE_ADMIN", "created_by": "admin@example.com",
				"state": "APPLICATION_KEY_STATE_ACTIVE", "permitted_ips.0": "10.0.0.1",
			},
		},
		{
			typeName: "reco_policies", listAttr: "policies", listPath: "/policies/list", errorTitle: "Error listing policies", paginated: true,
			seed: func(srv *fakereco.Server, n int) {
				for i := range n {
					srv.SeedPolicy(client.ThreatDetectionPolicy{
						Name: fmt.Sprintf("policy-%d", i), Severity: "HIGH", Status: "POLICY_STATUS_ON", PolicyType: "POLICY_TYPE_ITDR",
						Apps: []string{"GSUITE"}, Tags: []string{"t1"}, OpenAlerts: 3, AllAlerts: 7,
					})
				}
			},
			firstItem: map[string]string{
				"id": "policy-1", "name": "policy-0", "severity": "HIGH", "status": "POLICY_STATUS_ON", "policy_type": "POLICY_TYPE_ITDR",
				"apps.0": "GSUITE", "tags.0": "t1", "open_alerts": "3", "all_alerts": "7",
			},
		},
		{
			typeName: "reco_posture_checks", listAttr: "posture_checks", listPath: "/posture-checks/list", errorTitle: "Error listing posture checks", paginated: true,
			seed: func(srv *fakereco.Server, n int) {
				for i := range n {
					srv.SeedPostureCheck(client.PostureCheck{
						Name: fmt.Sprintf("check-%d", i), Description: "a check", Severity: "LOW", AppSource: "GSUITE_USERS_API",
						PolicyType: "POLICY_TYPE_POSTURE", Apps: []string{"GSUITE"}, Tags: []string{"t1"},
					})
				}
			},
			firstItem: map[string]string{
				"id": "posture-1", "name": "check-0", "description": "a check", "severity": "LOW", "app_source": "GSUITE_USERS_API",
				"policy_type": "POLICY_TYPE_POSTURE", "apps.0": "GSUITE", "tags.0": "t1",
			},
		},
		{
			typeName: "reco_integrations", listAttr: "integrations", listPath: "/integrations/list", errorTitle: "Error listing integrations", paginated: true,
			seed: func(srv *fakereco.Server, n int) {
				for i := range n {
					srv.SeedIntegration(client.Integration{
						Instance: client.AppInstance{ID: fmt.Sprintf("inst-%d", i)}, App: "GSUITE", Status: "CONNECTED",
						HasNewEndpoints: true, LastUpdatedBy: "admin@example.com", SeverityConfig: "DEFAULT",
					})
				}
			},
			firstItem: map[string]string{
				"instance_id": "inst-0", "app": "GSUITE", "status": "CONNECTED", "has_new_endpoints": "true",
				"last_updated_by": "admin@example.com", "severity_config": "DEFAULT",
			},
		},
	}
}

func (c dataSourceCase) config(srv *fakereco.Server) string {
	return unitProviderConfig(srv) + fmt.Sprintf("\ndata %q \"all\" {}\n", c.typeName)
}

func unitProviderConfig(srv *fakereco.Server) string {
	return fmt.Sprintf(`
provider "reco" {
  api_key  = %q
  base_url = %q
}
`, fakereco.APIKey, srv.URL)
}

func unitTest(t *testing.T, steps ...resource.TestStep) {
	t.Helper()
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: providerFactoriesDS(), Steps: steps})
}

func TestDataSources_listSeededItems(t *testing.T) {
	const seeded = 5
	for _, tc := range dataSourceCases() {
		t.Run(tc.typeName, func(t *testing.T) {
			if tc.paginated {
				t.Setenv("RECO_TEST_PAGE_SIZE", "2")
			}
			srv := fakereco.New(t)
			tc.seed(srv, seeded)

			addr := "data." + tc.typeName + ".all"
			checks := []resource.TestCheckFunc{
				resource.TestCheckResourceAttr(addr, tc.listAttr+".#", strconv.Itoa(seeded+tc.baseCount)),
			}
			for attr, want := range tc.firstItem {
				checks = append(checks, resource.TestCheckResourceAttr(addr, fmt.Sprintf("%s.%d.%s", tc.listAttr, tc.firstIndex, attr), want))
			}
			unitTest(t, resource.TestStep{Config: tc.config(srv), Check: resource.ComposeTestCheckFunc(checks...)})

			if tc.paginated {
				// 5 items at 2 per page need 3 requests per read; Terraform reads the data source more than once.
				if got := srv.CountRequests(http.MethodGet, tc.listPath); got < 3 || got%3 != 0 {
					t.Errorf("made %d list requests, want a multiple of 3 (pages of 2, 2, 1)", got)
				}
			}
		})
	}
}

func TestDataSources_emptyList(t *testing.T) {
	for _, tc := range dataSourceCases() {
		t.Run(tc.typeName, func(t *testing.T) {
			srv := fakereco.New(t)
			unitTest(t, resource.TestStep{
				Config: tc.config(srv),
				Check:  resource.TestCheckResourceAttr("data."+tc.typeName+".all", tc.listAttr+".#", strconv.Itoa(tc.baseCount)),
			})
		})
	}
}

func TestDataSources_apiErrorIsReported(t *testing.T) {
	for _, tc := range dataSourceCases() {
		t.Run(tc.typeName, func(t *testing.T) {
			srv := fakereco.New(t)
			srv.FailWith(http.MethodGet, tc.listPath, http.StatusForbidden, "no access")
			unitTest(t, resource.TestStep{
				Config:      tc.config(srv),
				ExpectError: regexp.MustCompile(regexp.QuoteMeta(tc.errorTitle) + `[\s\S]*HTTP 403`),
			})
		})
	}
}

func TestDataSources_seeCreatedResourcesImmediately(t *testing.T) {
	srv := fakereco.New(t)
	srv.SeedUser(client.RecoUser{EmailAddress: "first@example.com", Name: "First"})
	cfg := unitProviderConfig(srv) + `data "reco_users" "all" {}
`
	unitTest(t,
		resource.TestStep{Config: cfg, Check: resource.TestCheckResourceAttr("data.reco_users.all", "users.#", "1")},
		resource.TestStep{
			PreConfig: func() { srv.SeedUser(client.RecoUser{EmailAddress: "second@example.com", Name: "Second"}) },
			Config:    cfg,
			Check:     resource.TestCheckResourceAttr("data.reco_users.all", "users.#", "2"),
		},
	)
}
