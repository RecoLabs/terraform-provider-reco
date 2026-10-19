package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScimQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"simple", `"simple"`},
		{`with"quote`, `"with\"quote"`},
		{`with\slash`, `"with\\slash"`},
		{`"both`, `"\"both"`},
		{"", `""`},
	}
	for _, tc := range tests {
		if got := scimQuote(tc.in); got != tc.want {
			t.Errorf("scimQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	nfe := &NotFoundError{Message: "not found"}
	if !IsNotFound(nfe) {
		t.Fatal("expected IsNotFound true for *NotFoundError")
	}
	if !IsNotFound(fmt.Errorf("wrap: %w", nfe)) {
		t.Fatal("expected IsNotFound true for wrapped *NotFoundError")
	}
	if IsNotFound(fmt.Errorf("unrelated error")) {
		t.Fatal("expected IsNotFound false for unrelated error")
	}
	if IsNotFound(nil) {
		t.Fatal("expected IsNotFound false for nil")
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL, "key", "test")
}

func TestComplianceFrameworkLifecycle(t *testing.T) {
	var upserted UpsertComplianceFrameworkRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/external-api/compliance-frameworks/create":
			_, _ = w.Write([]byte(`{"id":"fw-1"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/external-api/compliance-frameworks/upsert":
			_ = json.NewDecoder(r.Body).Decode(&upserted)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/external-api/compliance-frameworks/list":
			if got := r.URL.Query().Get("filters"); got != `id eq "fw-1"` {
				t.Errorf("unexpected filter %q", got)
			}
			_, _ = w.Write([]byte(`{"totalResults":"1","itemsPerPage":"1","complianceFrameworks":[{"id":"fw-1","title":"T","isCustom":true,"isActive":true}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/external-api/compliance-frameworks/fw-1":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()

	id, err := c.CreateComplianceFramework(ctx, &ComplianceFrameworkDefinition{Title: "T"})
	if err != nil || id != "fw-1" {
		t.Fatalf("create: id=%q err=%v", id, err)
	}
	if err := c.UpsertComplianceFramework(ctx, &UpsertComplianceFrameworkRequest{Framework: ComplianceFrameworkInput{ID: id, Title: "T"}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if upserted.Controls == nil {
		t.Error("upsert must send an explicit controls list, not null")
	}
	fw, err := c.GetComplianceFrameworkByID(ctx, "fw-1")
	if err != nil || !fw.IsCustom || !fw.IsActive {
		t.Fatalf("get: fw=%+v err=%v", fw, err)
	}
	if err := c.DeleteComplianceFramework(ctx, "fw-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestGetComplianceFrameworkByID_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totalResults":"0","itemsPerPage":"0","complianceFrameworks":[]}`))
	})
	if _, err := c.GetComplianceFrameworkByID(context.Background(), "missing"); !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestDashboardSaveAndGet(t *testing.T) {
	var saved SaveDashboardRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/external-api/dashboards/save":
			_ = json.NewDecoder(r.Body).Decode(&saved)
			_, _ = w.Write([]byte(`{"dashboard":{"id":"d-1","name":"N","spec":"{}","visibility":"DASHBOARD_VISIBILITY_PRIVATE","folderPath":"/"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/external-api/dashboards/list":
			if got := r.URL.Query()["ids"]; len(got) != 1 {
				t.Errorf("expected exactly one ids parameter, got %v", got)
			}
			_, _ = w.Write([]byte(`{"dashboards":[{"id":"d-1","name":"N","spec":"{}"}]}`))
		default:
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()

	d, err := c.SaveDashboard(ctx, &SaveDashboardRequest{Name: "N", Spec: "{}"})
	if err != nil || d.ID != "d-1" {
		t.Fatalf("save: d=%+v err=%v", d, err)
	}
	if saved.ID != "" || saved.Name != "N" {
		t.Errorf("unexpected save body %+v", saved)
	}
	if _, err := c.GetDashboardByID(ctx, "d-1"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if _, err := c.GetDashboardByID(ctx, "other"); !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}
