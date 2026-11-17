//nolint:goconst // tests repeat field names and IDs on purpose
package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/recolabs/terraform-provider-reco/internal/client"
)

type recordedRequest struct {
	method string
	path   string
	query  string
	body   map[string]any
}

// newRecordingServer answers every request with status and respBody and records
// what it received.
func newRecordingServer(t *testing.T, status int, respBody string) (*client.Client, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method = r.Method
		rec.path = r.URL.EscapedPath()
		rec.query = r.URL.Query().Get("filters")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &rec.body); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "test-key", "test"), rec
}

func TestCreateComplianceFramework(t *testing.T) {
	c, rec := newRecordingServer(t, http.StatusOK, `{"id":"7d6a1f5e-0000-4000-8000-000000000001"}`)

	id, err := c.CreateComplianceFramework(context.Background(), &client.ComplianceFrameworkDefinition{
		Title:       "Internal Controls",
		Description: "Our controls",
		Version:     "2026.1",
		Type:        "internal",
		IsActive:    true,
	})
	if err != nil {
		t.Fatalf("CreateComplianceFramework: %v", err)
	}
	if id != "7d6a1f5e-0000-4000-8000-000000000001" {
		t.Errorf("id = %q", id)
	}
	if rec.method != http.MethodPost || rec.path != "/api/v1/external-api/compliance-frameworks/create" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	want := map[string]any{"definition": map[string]any{
		"title": "Internal Controls", "description": "Our controls", "version": "2026.1", "type": "internal", "isActive": true,
	}}
	if !reflect.DeepEqual(rec.body, want) {
		t.Errorf("body = %v, want %v", rec.body, want)
	}
}

func TestCreateComplianceFramework_sendsExplicitInactive(t *testing.T) {
	c, rec := newRecordingServer(t, http.StatusOK, `{"id":"x"}`)
	if _, err := c.CreateComplianceFramework(context.Background(), &client.ComplianceFrameworkDefinition{Title: "T"}); err != nil {
		t.Fatalf("CreateComplianceFramework: %v", err)
	}
	def, _ := rec.body["definition"].(map[string]any)
	if active, ok := def["isActive"]; !ok || active != false {
		t.Errorf("isActive = %v (present=%v), want an explicit false", active, ok)
	}
}

func TestUpsertComplianceFramework(t *testing.T) {
	tests := []struct {
		name         string
		controls     []client.ComplianceControl
		wantControls any
	}{
		{
			name:         "nil controls are sent as an empty list",
			controls:     nil,
			wantControls: []any{},
		},
		{
			name: "all control fields",
			controls: []client.ComplianceControl{{
				ControlID:        "CC6.1",
				Title:            "Access",
				Description:      "Restrict access",
				ParentControlID:  "CC6",
				RelatedPolicyIDs: []string{"11111111-1111-4111-8111-111111111111"},
				Manual:           true,
				Skipped:          false,
			}},
			wantControls: []any{map[string]any{
				"controlId":        "CC6.1",
				"title":            "Access",
				"description":      "Restrict access",
				"parentControlId":  "CC6",
				"relatedPolicyIds": []any{"11111111-1111-4111-8111-111111111111"},
				"manual":           true,
				"skipped":          false,
			}},
		},
		{
			name:     "control with only an ID",
			controls: []client.ComplianceControl{{ControlID: "A.1"}},
			wantControls: []any{map[string]any{
				"controlId": "A.1", "manual": false, "skipped": false,
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newRecordingServer(t, http.StatusOK, `{}`)
			err := c.UpsertComplianceFramework(context.Background(), &client.UpsertComplianceFrameworkRequest{
				Framework: client.ComplianceFrameworkInput{ID: "fw-id", Title: "T", Version: "1"},
				Controls:  tc.controls,
			})
			if err != nil {
				t.Fatalf("UpsertComplianceFramework: %v", err)
			}
			if rec.method != http.MethodPut || rec.path != "/api/v1/external-api/compliance-frameworks/upsert" {
				t.Errorf("request = %s %s", rec.method, rec.path)
			}
			wantFramework := map[string]any{"id": "fw-id", "title": "T", "version": "1", "isActive": false}
			if !reflect.DeepEqual(rec.body["framework"], wantFramework) {
				t.Errorf("framework = %v, want %v", rec.body["framework"], wantFramework)
			}
			if !reflect.DeepEqual(rec.body["controls"], tc.wantControls) {
				t.Errorf("controls = %v, want %v", rec.body["controls"], tc.wantControls)
			}
		})
	}
}

func TestGetComplianceFrameworkByID(t *testing.T) {
	const respBody = `{
		"totalResults": "1", "itemsPerPage": "1",
		"complianceFrameworks": [{
			"id": "fw-id", "title": "T", "description": "D", "version": "2", "type": "custom",
			"isCustom": true, "isActive": true,
			"createdAt": "2026-11-01T10:00:00Z", "updatedAt": "2026-11-02T10:00:00Z"
		}]
	}`
	c, rec := newRecordingServer(t, http.StatusOK, respBody)

	got, err := c.GetComplianceFrameworkByID(context.Background(), `fw"id`)
	if err != nil {
		t.Fatalf("GetComplianceFrameworkByID: %v", err)
	}
	want := &client.ComplianceFramework{
		ID: "fw-id", Title: "T", Description: "D", Version: "2", Type: "custom",
		IsCustom: true, IsActive: true, CreatedAt: "2026-11-01T10:00:00Z", UpdatedAt: "2026-11-02T10:00:00Z",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("framework = %+v, want %+v", got, want)
	}
	if rec.method != http.MethodGet || rec.path != "/api/v1/external-api/compliance-frameworks/list" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	if !strings.HasPrefix(rec.query, `id eq "fw\"id" and title ne "reco-tf-`) {
		t.Errorf("filters = %q, want the id quoted as a SCIM string, then the cache-busting clause", rec.query)
	}
}

func TestGetComplianceFrameworkByID_notFound(t *testing.T) {
	c, _ := newRecordingServer(t, http.StatusOK, `{"totalResults":"0","itemsPerPage":"0"}`)
	_, err := c.GetComplianceFrameworkByID(context.Background(), "missing")
	if !client.IsNotFound(err) {
		t.Fatalf("err = %v, want a NotFoundError", err)
	}
}

func TestDeleteComplianceFramework(t *testing.T) {
	t.Run("deletes by escaped ID", func(t *testing.T) {
		c, rec := newRecordingServer(t, http.StatusOK, `{}`)
		if err := c.DeleteComplianceFramework(context.Background(), "a/b"); err != nil {
			t.Fatalf("DeleteComplianceFramework: %v", err)
		}
		if rec.method != http.MethodDelete {
			t.Errorf("method = %s", rec.method)
		}
		if rec.path != "/api/v1/external-api/compliance-frameworks/a%2Fb" {
			t.Errorf("path = %q, want the ID path-escaped", rec.path)
		}
	})
	t.Run("404 is a NotFoundError", func(t *testing.T) {
		c, _ := newRecordingServer(t, http.StatusNotFound, `{"message":"gone"}`)
		if err := c.DeleteComplianceFramework(context.Background(), "fw-id"); !client.IsNotFound(err) {
			t.Fatalf("err = %v, want a NotFoundError", err)
		}
	})
	t.Run("server errors are returned", func(t *testing.T) {
		c, _ := newRecordingServer(t, http.StatusForbidden, `{"message":"no"}`)
		err := c.DeleteComplianceFramework(context.Background(), "fw-id")
		if err == nil || client.IsNotFound(err) {
			t.Fatalf("err = %v, want a non-NotFound error", err)
		}
	})
}

func TestListAllComplianceFrameworks_paginates(t *testing.T) {
	pages := map[string]string{
		"1": `{"totalResults":"3","itemsPerPage":"2","complianceFrameworks":[{"id":"a","title":"A"},{"id":"b","title":"B","isCustom":true}]}`,
		"3": `{"totalResults":"3","itemsPerPage":"1","complianceFrameworks":[{"id":"c","title":"C"}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Query().Get("startIndex")]
		if !ok {
			t.Errorf("unexpected startIndex %q", r.URL.Query().Get("startIndex"))
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	got, err := client.New(srv.URL, "test-key", "test").WithPageSize(2).ListAllComplianceFrameworks(context.Background())
	if err != nil {
		t.Fatalf("ListAllComplianceFrameworks: %v", err)
	}
	ids := make([]string, 0, len(got))
	for _, f := range got {
		ids = append(ids, f.ID)
	}
	if !reflect.DeepEqual(ids, []string{"a", "b", "c"}) {
		t.Errorf("ids = %v, want [a b c]", ids)
	}
	if !got[1].IsCustom || got[0].IsCustom {
		t.Errorf("isCustom not decoded: %+v", got)
	}
}

// No two reads may send the same query, so the API's list cache cannot serve them (see uncachedComplianceFilter).
func TestComplianceFrameworkReads_neverRepeatAQuery(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"totalResults":"1","itemsPerPage":"1","complianceFrameworks":[{"id":"fw-id","title":"T"}]}`))
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL, "test-key", "test")

	for range 3 {
		if _, err := c.GetComplianceFrameworkByID(context.Background(), "fw-id"); err != nil {
			t.Fatalf("GetComplianceFrameworkByID: %v", err)
		}
		if _, err := c.ListAllComplianceFrameworks(context.Background()); err != nil {
			t.Fatalf("ListAllComplianceFrameworks: %v", err)
		}
	}
	seen := map[string]bool{}
	for _, q := range queries {
		if seen[q] {
			t.Errorf("query %q was sent twice", q)
		}
		seen[q] = true
	}
	if len(queries) != 6 {
		t.Errorf("got %d requests, want 6", len(queries))
	}
}
