package resources_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/recolabs/terraform-provider-reco/internal/client"
)

// fakeCustomAPI is a minimal in-memory implementation of the compliance framework and dashboard
// endpoints, including the behaviours the provider has to cope with: controls are never returned,
// isActive is only honoured on creation, spec is re-serialized, and dashboards cannot be deleted.
type fakeCustomAPI struct {
	*httptest.Server
	mu         sync.Mutex
	seq        int
	frameworks map[string]client.ComplianceFramework
	controls   map[string][]client.ComplianceControl
	dashboards map[string]client.Dashboard
}

func newFakeCustomAPI(t *testing.T) *fakeCustomAPI {
	t.Helper()
	f := &fakeCustomAPI{
		frameworks: map[string]client.ComplianceFramework{},
		controls:   map[string][]client.ComplianceControl{},
		dashboards: map[string]client.Dashboard{},
	}
	f.Server = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)

	// The provider runs in-process and requires an https base_url, so trust the test certificate.
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected default transport %T", http.DefaultTransport)
	}
	orig := transport.TLSClientConfig
	pool := x509.NewCertPool()
	pool.AddCert(f.Certificate())
	transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	t.Cleanup(func() { transport.TLSClientConfig = orig })
	return f
}

func (f *fakeCustomAPI) providerConfig() string {
	return fmt.Sprintf("provider \"reco\" {\n  api_key  = \"test-key\"\n  base_url = %q\n}\n", f.URL)
}

func (f *fakeCustomAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/external-api")
	reply := func(v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	fail := func(code int, msg string) { http.Error(w, msg, code) }

	switch {
	case r.Method == http.MethodPost && path == "/compliance-frameworks/create":
		var req client.CreateComplianceFrameworkRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.seq++
		id := fmt.Sprintf("fw-%d", f.seq)
		d := req.Definition
		f.frameworks[id] = client.ComplianceFramework{ID: id, Title: d.Title, Description: d.Description, Version: d.Version, Type: d.Type, IsCustom: true, IsActive: d.IsActive}
		reply(client.CreateComplianceFrameworkResponse{ID: id})
	case r.Method == http.MethodPut && path == "/compliance-frameworks/upsert":
		var req client.UpsertComplianceFrameworkRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		fw, ok := f.frameworks[req.Framework.ID]
		if !ok {
			fail(http.StatusNotFound, "framework not found")
			return
		}
		fw.Title, fw.Description, fw.Version, fw.Type = req.Framework.Title, req.Framework.Description, req.Framework.Version, req.Framework.Type
		f.frameworks[fw.ID] = fw // IsActive intentionally untouched
		f.controls[fw.ID] = req.Controls
		reply(struct{}{})
	case r.Method == http.MethodGet && path == "/compliance-frameworks/list":
		out := []client.ComplianceFramework{}
		for _, fw := range f.frameworks {
			if strings.Contains(r.URL.Query().Get("filters"), `"`+fw.ID+`"`) {
				out = append(out, fw)
			}
		}
		reply(map[string]any{"totalResults": fmt.Sprint(len(out)), "itemsPerPage": fmt.Sprint(len(out)), "complianceFrameworks": out})
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/compliance-frameworks/"):
		id := strings.TrimPrefix(path, "/compliance-frameworks/")
		if _, ok := f.frameworks[id]; !ok {
			fail(http.StatusNotFound, "framework not found")
			return
		}
		delete(f.frameworks, id)
		delete(f.controls, id)
		reply(struct{}{})
	case r.Method == http.MethodPost && path == "/dashboards/save":
		var req client.SaveDashboardRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		d, exists := f.dashboards[req.ID]
		if req.ID == "" {
			f.seq++
			req.ID = fmt.Sprintf("dash-%d", f.seq)
			d = client.Dashboard{ID: req.ID, Visibility: "DASHBOARD_VISIBILITY_PRIVATE", FolderPath: "/", CreatedBy: "tf@example.com"}
		} else if !exists {
			d = client.Dashboard{ID: req.ID, Visibility: "DASHBOARD_VISIBILITY_PRIVATE", FolderPath: "/", CreatedBy: "tf@example.com"}
		}
		created := "2026-01-01T00:00:00Z"
		d.Name = req.Name
		d.Spec = reserialize(req.Spec)
		d.CreatedAt = &created
		if req.Visibility != "" {
			d.Visibility = req.Visibility
		}
		if req.FolderPath != "" {
			d.FolderPath = req.FolderPath
		}
		f.dashboards[d.ID] = d
		reply(client.SaveDashboardResponse{Dashboard: d})
	case r.Method == http.MethodGet && path == "/dashboards/list":
		out := []client.Dashboard{}
		for _, id := range r.URL.Query()["ids"] {
			if d, ok := f.dashboards[id]; ok {
				out = append(out, d)
			}
		}
		reply(map[string]any{"dashboards": out})
	default:
		fail(http.StatusNotFound, "unknown endpoint "+r.Method+" "+path)
	}
}

// reserialize mimics a server that stores spec in its own canonical, indented form.
func reserialize(spec string) string {
	var v any
	if json.Unmarshal([]byte(spec), &v) != nil {
		return spec
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func (f *fakeCustomAPI) frameworkControls(id string) []client.ComplianceControl {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.controls[id]
}

func TestCustomComplianceFramework_lifecycle(t *testing.T) {
	api := newFakeCustomAPI(t)
	cfg := func(description, controls string) string {
		return api.providerConfig() + fmt.Sprintf(`
resource "reco_custom_compliance_framework" "test" {
  title       = "Unit Framework"
  description = %q
  version     = "1.0"
  %s
}
`, description, controls)
	}
	oneControl := `controls = [{
    control_id         = "C-1"
    title              = "Control 1"
    related_policy_ids = ["11111111-1111-1111-1111-111111111111"]
    manual             = true
  }]`
	twoControls := `controls = [
    { control_id = "C-1", title = "Control 1" },
    { control_id = "C-2", title = "Control 2", parent_control_id = "C-1" },
  ]`

	customUnitTest(t,
		resource.TestStep{
			Config: cfg("first", oneControl),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("reco_custom_compliance_framework.test", "id"),
				resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "description", "first"),
				resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "is_active", "false"),
				resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "controls.#", "1"),
			),
		},
		resource.TestStep{
			Config:           cfg("second", twoControls),
			ConfigPlanChecks: customExpectAction("reco_custom_compliance_framework.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "description", "second"),
				resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "controls.#", "2"),
				func(s *terraform.State) error {
					id := s.RootModule().Resources["reco_custom_compliance_framework.test"].Primary.ID
					c := api.frameworkControls(id)
					if len(c) != 2 || c[1].ParentControlID != "C-1" {
						return fmt.Errorf("API controls = %+v", c)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			ResourceName:            "reco_custom_compliance_framework.test",
			ImportState:             true,
			ImportStateVerify:       true,
			ImportStateVerifyIgnore: []string{"controls"},
		},
	)
	if n := len(api.frameworks); n != 0 {
		t.Errorf("framework should be deleted on destroy, %d left", n)
	}
}

func TestCustomComplianceFramework_isActiveForcesReplacement(t *testing.T) {
	api := newFakeCustomAPI(t)
	cfg := func(active string) string {
		return api.providerConfig() + fmt.Sprintf(`
resource "reco_custom_compliance_framework" "test" {
  title     = "Active Framework"
  is_active = %s
}
`, active)
	}
	customUnitTest(t,
		resource.TestStep{
			Config: cfg("true"),
			Check:  resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "is_active", "true"),
		},
		resource.TestStep{
			Config:           cfg("false"),
			ConfigPlanChecks: customExpectAction("reco_custom_compliance_framework.test", plancheck.ResourceActionDestroyBeforeCreate),
			Check:            resource.TestCheckResourceAttr("reco_custom_compliance_framework.test", "is_active", "false"),
		},
	)
}

func TestCustomComplianceFramework_recreatedWhenDeletedOutOfBand(t *testing.T) {
	api := newFakeCustomAPI(t)
	config := api.providerConfig() + `
resource "reco_custom_compliance_framework" "test" {
  title = "Drift Framework"
}
`
	customUnitTest(t,
		resource.TestStep{Config: config},
		resource.TestStep{
			PreConfig: func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				clear(api.frameworks)
			},
			Config:           config,
			ConfigPlanChecks: customExpectAction("reco_custom_compliance_framework.test", plancheck.ResourceActionCreate),
		},
	)
}

func TestCustomDashboard_lifecycle(t *testing.T) {
	api := newFakeCustomAPI(t)
	cfg := func(name, visibility string) string {
		return api.providerConfig() + fmt.Sprintf(`
resource "reco_custom_dashboard" "test" {
  name       = %q
  visibility = %q
  spec       = jsonencode({ widgets = ["DASHBOARD_WIDGET_TOTAL_SAAS_APPS"], columns = 2 })
}
`, name, visibility)
	}
	customUnitTest(t,
		resource.TestStep{
			Config: cfg("Unit Dashboard", "DASHBOARD_VISIBILITY_PRIVATE"),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("reco_custom_dashboard.test", "id"),
				resource.TestCheckResourceAttr("reco_custom_dashboard.test", "folder_path", "/"),
				resource.TestCheckResourceAttr("reco_custom_dashboard.test", "created_by", "tf@example.com"),
			),
		},
		// The server re-serializes spec; an unchanged config must still produce an empty plan.
		resource.TestStep{
			Config:   cfg("Unit Dashboard", "DASHBOARD_VISIBILITY_PRIVATE"),
			PlanOnly: true,
		},
		resource.TestStep{
			Config:           cfg("Unit Dashboard Renamed", "DASHBOARD_VISIBILITY_SHARED_READ"),
			ConfigPlanChecks: customExpectAction("reco_custom_dashboard.test", plancheck.ResourceActionUpdate),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("reco_custom_dashboard.test", "name", "Unit Dashboard Renamed"),
				resource.TestCheckResourceAttr("reco_custom_dashboard.test", "visibility", "DASHBOARD_VISIBILITY_SHARED_READ"),
			),
		},
		resource.TestStep{
			ResourceName:            "reco_custom_dashboard.test",
			ImportState:             true,
			ImportStateVerify:       true,
			ImportStateVerifyIgnore: []string{"spec"},
		},
	)
	if n := len(api.dashboards); n != 1 {
		t.Errorf("dashboard should remain in Reco after destroy (API has no delete), found %d", n)
	}
}

func TestCustomDashboard_rejectsInvalidInput(t *testing.T) {
	api := newFakeCustomAPI(t)
	customUnitTest(t,
		resource.TestStep{
			Config: api.providerConfig() + `
resource "reco_custom_dashboard" "test" {
  name = "Bad"
  spec = "not json"
}
`,
			ExpectError: regexp.MustCompile("Invalid JSON"),
		},
		resource.TestStep{
			Config: api.providerConfig() + `
resource "reco_custom_dashboard" "test" {
  name       = "Bad"
  spec       = "{}"
  visibility = "PUBLIC"
}
`,
			ExpectError: regexp.MustCompile("visibility"),
		},
	)
}

func customUnitTest(t *testing.T, steps ...resource.TestStep) {
	t.Helper()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps:                    steps,
	})
}

func customExpectAction(addr string, action plancheck.ResourceActionType) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, action)},
	}
}
