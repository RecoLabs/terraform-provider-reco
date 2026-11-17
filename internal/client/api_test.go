//nolint:goconst // tests repeat resource addresses and API field names on purpose
package client_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflogtest"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

func noBackoff(int) time.Duration { return 0 }

func newFakeClient(t *testing.T) (*client.Client, *fakereco.Server) {
	t.Helper()
	srv := fakereco.New(t)
	return client.New(srv.URL, fakereco.APIKey, "test", client.WithRetryBackoff(noBackoff)), srv
}

func TestClient_sendsAuthAndUserAgent(t *testing.T) {
	var gotAuth, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotUA = r.Header.Get("Authorization"), r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"roles":[]}`))
	}))
	defer srv.Close()

	if _, err := client.New(srv.URL, "k-123", "9.9.9").ListAllRoles(t.Context()); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer k-123" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer k-123")
	}
	if gotUA != "reco-tf-provider/9.9.9" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "reco-tf-provider/9.9.9")
	}
}

func TestClient_badAPIKeyIsRejected(t *testing.T) {
	srv := fakereco.New(t)
	_, err := client.New(srv.URL, "wrong", "test").ListAllRoles(t.Context())
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("want HTTP 401 error, got %v", err)
	}
}

func TestClient_users(t *testing.T) {
	c, srv := newFakeClient(t)
	ctx := t.Context()

	created, err := c.CreateUser(ctx, &client.RecoUser{EmailAddress: "a@example.com", Name: "A", UserRoles: []string{"ROLE_ADMIN"}})
	if err != nil {
		t.Fatal(err)
	}
	if created.UserID == "" {
		t.Fatal("CreateUser returned no userId")
	}

	byID, err := c.GetUserByID(ctx, created.UserID)
	if err != nil || byID.EmailAddress != "a@example.com" {
		t.Fatalf("GetUserByID = %+v, %v", byID, err)
	}
	byEmail, err := c.GetUserByEmail(ctx, "a@example.com")
	if err != nil || byEmail.UserID != created.UserID {
		t.Fatalf("GetUserByEmail = %+v, %v", byEmail, err)
	}

	err = c.UpdateUser(ctx, &client.UpdateRecoUserRequest{
		User:             client.RecoUser{UserID: created.UserID, EmailAddress: "a@example.com", Name: "Renamed", UserRoles: []string{"ROLE_ADMIN"}},
		EnablementStatus: "disabled",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := c.GetUserByID(ctx, created.UserID)
	if err != nil || updated.Name != "Renamed" || updated.EnablementStatus != "disabled" {
		t.Fatalf("after update: %+v, %v", updated, err)
	}

	if err := c.DeleteUser(ctx, created.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetUserByID(ctx, created.UserID); !client.IsNotFound(err) {
		t.Fatalf("GetUserByID after delete: want not found, got %v", err)
	}
	if err := c.DeleteUser(ctx, created.UserID); !client.IsNotFound(err) {
		t.Fatalf("second DeleteUser: want not found, got %v", err)
	}
	if n := len(srv.Users()); n != 0 {
		t.Fatalf("%d users left", n)
	}
}

func TestClient_userLookupEscapesFilterValues(t *testing.T) {
	c, srv := newFakeClient(t)
	odd := `we"ird\name@example.com`
	srv.SeedUser(client.RecoUser{EmailAddress: odd, Name: "Odd"})
	srv.SeedUser(client.RecoUser{EmailAddress: "other@example.com", Name: "Other"})

	got, err := c.GetUserByEmail(t.Context(), odd)
	if err != nil || got.Name != "Odd" {
		t.Fatalf("GetUserByEmail = %+v, %v", got, err)
	}
}

func TestClient_ambiguousLookups(t *testing.T) {
	c, srv := newFakeClient(t)
	srv.SeedUser(client.RecoUser{EmailAddress: "dup@example.com"})
	srv.SeedUser(client.RecoUser{EmailAddress: "dup@example.com"})
	srv.SeedRole(client.RecoRole{Name: "dup-role"})
	srv.SeedRole(client.RecoRole{Name: "dup-role"})

	if _, err := c.GetUserByEmail(t.Context(), "dup@example.com"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("GetUserByEmail: want ambiguous error, got %v", err)
	}
	if _, err := c.GetRoleByName(t.Context(), "dup-role"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("GetRoleByName: want ambiguous error, got %v", err)
	}
}

func TestClient_roles(t *testing.T) {
	c, _ := newFakeClient(t)
	ctx := t.Context()

	role := &client.RecoCustomRole{Name: "r1", Description: "d", Permissions: []string{"PERM_ALERTS_READ"}, Resources: []client.ResourcePermission{}}
	if err := c.CreateRole(ctx, role); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetRoleByName(ctx, "r1")
	if err != nil || got.Description != "d" || got.Type != "USER_ROLE_TYPE_CUSTOM" {
		t.Fatalf("GetRoleByName = %+v, %v", got, err)
	}

	role.Name, role.Description = "r1-renamed", "d2"
	if err := c.UpdateRole(ctx, role, "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetRoleByName(ctx, "r1"); !client.IsNotFound(err) {
		t.Fatalf("old name should be gone, got %v", err)
	}

	all, err := c.ListAllRoles(ctx)
	if err != nil || len(all) != 3 { // two system roles plus ours
		t.Fatalf("ListAllRoles = %d roles, %v", len(all), err)
	}

	if err := c.DeleteRole(ctx, "r1-renamed"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteRole(ctx, "r1-renamed"); !client.IsNotFound(err) {
		t.Fatalf("second DeleteRole: want not found, got %v", err)
	}
}

func TestClient_postureChecks(t *testing.T) {
	c, _ := newFakeClient(t)
	ctx := t.Context()

	def := &client.PostureCheckDefinition{Title: "A posture check", DataSource: "GSUITE_USERS_API", ViolationRiskLevel: "LOW", Tags: []string{"x"}}
	id, err := c.CreatePostureCheck(ctx, def)
	if err != nil || id == "" {
		t.Fatalf("CreatePostureCheck = %q, %v", id, err)
	}
	got, err := c.GetPostureCheckByID(ctx, id)
	if err != nil || got.Name != "A posture check" || got.Severity != "LOW" {
		t.Fatalf("GetPostureCheckByID = %+v, %v", got, err)
	}

	def.Title = "Renamed posture check"
	if err := c.UpdatePostureCheck(ctx, id, def); err != nil {
		t.Fatal(err)
	}
	all, err := c.ListAllPostureChecks(ctx)
	if err != nil || len(all) != 1 || all[0].Name != "Renamed posture check" {
		t.Fatalf("ListAllPostureChecks = %+v, %v", all, err)
	}

	if err := c.DeletePostureCheck(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetPostureCheckByID(ctx, id); !client.IsNotFound(err) {
		t.Fatalf("after delete: want not found, got %v", err)
	}
	if err := c.UpdatePostureCheck(ctx, id, def); !client.IsNotFound(err) {
		t.Fatalf("update after delete: want not found, got %v", err)
	}
}

func TestClient_threatDetectionPolicies(t *testing.T) {
	c, srv := newFakeClient(t)
	ctx := t.Context()

	def := &client.ThreatDetectionPolicyDefinition{Title: "A policy title", DataSource: "GSUITE_ADMIN_AUDIT_LOG_API", ViolationRiskLevel: "HIGH"}
	id, err := c.CreateThreatDetectionPolicy(ctx, def, map[string]string{
		"instance-off": "POLICY_STATUS_OFF",
		"instance-on":  "POLICY_STATUS_ON",
	})
	if err != nil || id == "" {
		t.Fatalf("CreateThreatDetectionPolicy = %q, %v", id, err)
	}
	got, err := c.GetThreatDetectionPolicyByID(ctx, id)
	if err != nil || got.Name != "A policy title" || got.Severity != "HIGH" {
		t.Fatalf("GetThreatDetectionPolicyByID = %+v, %v", got, err)
	}
	if status := srv.PolicyInstanceStatus(id); status["instance-off"] != "POLICY_STATUS_OFF" || status["instance-on"] != "POLICY_STATUS_ON" {
		t.Fatalf("instance status after create = %v", status)
	}

	def.Title = "Renamed policy title"
	if err := c.UpdateThreatDetectionPolicy(ctx, id, def); err != nil {
		t.Fatal(err)
	}
	if status := srv.PolicyInstanceStatus(id); status["instance-off"] != "POLICY_STATUS_OFF" || status["instance-on"] != "POLICY_STATUS_ON" {
		t.Fatalf("update must keep instance status from create, got %v", status)
	}
	all, err := c.ListAllPolicies(ctx)
	if err != nil || len(all) != 1 || all[0].Name != "Renamed policy title" {
		t.Fatalf("ListAllPolicies = %+v, %v", all, err)
	}

	if err := c.DeleteThreatDetectionPolicy(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetThreatDetectionPolicyByID(ctx, id); !client.IsNotFound(err) {
		t.Fatalf("after delete: want not found, got %v", err)
	}
}

func TestClient_threatDetectionExclusions(t *testing.T) {
	c, _ := newFakeClient(t)
	ctx := t.Context()

	id, err := c.CreateThreatDetectionExclusion(ctx, &client.CreateThreatDetectionExclusionRequest{
		Definition: client.ThreatDetectionExclusionDefinition{
			Name:       "Trusted actor",
			Expression: "true",
			IsGlobal:   true,
		},
		State: client.ExclusionStateOnPermanent,
	})
	if err != nil || id == "" {
		t.Fatalf("CreateThreatDetectionExclusion = %q, %v", id, err)
	}
	got, err := c.GetThreatDetectionExclusionByID(ctx, id)
	if err != nil || got.Name != "Trusted actor" || got.State != client.ExclusionStateOnPermanent || got.Description != "" || got.ValidUntil != nil {
		t.Fatalf("GetThreatDetectionExclusionByID = %+v, %v", got, err)
	}

	if err := c.UpdateThreatDetectionExclusion(ctx, &client.UpdateThreatDetectionExclusionRequest{
		ID: id,
		Definition: client.ThreatDetectionExclusionDefinition{
			Name:        "Trusted actor renamed",
			Description: "maintenance",
			Expression:  "true",
			IsGlobal:    true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetThreatDetectionExclusionByID(ctx, id)
	if err != nil || got.Name != "Trusted actor renamed" || got.State != client.ExclusionStateOnPermanent || got.Description != "maintenance" {
		t.Fatalf("after update = %+v, %v", got, err)
	}

	until := "2099-01-01T00:00:00Z"
	if err := c.SetThreatDetectionExclusionState(ctx, client.SetThreatDetectionExclusionStateRequest{
		ID: id, State: client.ExclusionStateOnTemporary,
	}); err == nil {
		t.Fatal("ON_TEMPORARY without valid_until should fail")
	}
	if err := c.SetThreatDetectionExclusionState(ctx, client.SetThreatDetectionExclusionStateRequest{
		ID: id, State: client.ExclusionStateOnTemporary, ValidUntil: &until,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetThreatDetectionExclusionByID(ctx, id)
	if err != nil || got.State != client.ExclusionStateOnTemporary || got.ValidUntil == nil || *got.ValidUntil == until {
		t.Fatalf("temporary state = %+v, %v", got, err)
	}

	if err := c.SetThreatDetectionExclusionState(ctx, client.SetThreatDetectionExclusionStateRequest{
		ID: id, State: client.ExclusionStateOff, ValidUntil: &until,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetThreatDetectionExclusionByID(ctx, id)
	if err != nil || got.State != client.ExclusionStateOff || got.ValidUntil != nil {
		t.Fatalf("off state = %+v, %v", got, err)
	}

	all, err := c.ListAllExclusions(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("ListAllExclusions = %+v, %v", all, err)
	}
	if err := c.DeleteThreatDetectionExclusion(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetThreatDetectionExclusionByID(ctx, id); !client.IsNotFound(err) {
		t.Fatalf("after delete: want not found, got %v", err)
	}
	if err := c.DeleteThreatDetectionExclusion(ctx, id); !client.IsNotFound(err) {
		t.Fatalf("second delete: want not found, got %v", err)
	}
}

func TestClient_apiKeys(t *testing.T) {
	c, _ := newFakeClient(t)
	ctx := t.Context()

	created, err := c.CreateApiKey(ctx, client.CreateApiKeyRequest{Name: "key-1", Role: "ROLE_SECURITY_VIEWER"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" || created.Key.ID == "" {
		t.Fatalf("CreateApiKey = %+v", created)
	}
	if _, err := c.CreateApiKey(ctx, client.CreateApiKeyRequest{Name: "key-1", Role: "ROLE_SECURITY_VIEWER"}); err == nil {
		t.Fatal("duplicate key name should fail")
	}

	got, err := c.GetApiKeyByID(ctx, created.Key.ID)
	if err != nil || got.Name != "key-1" {
		t.Fatalf("GetApiKeyByID = %+v, %v", got, err)
	}
	updated, err := c.UpdateApiKey(ctx, &client.UpdateApiKeyRequest{ID: created.Key.ID, Name: "key-2", Role: "ROLE_ADMIN", PermittedIps: []string{"10.0.0.1"}})
	if err != nil || updated.Name != "key-2" || updated.Role != "ROLE_ADMIN" {
		t.Fatalf("UpdateApiKey = %+v, %v", updated, err)
	}
	retired, err := c.UpdateApiKey(ctx, &client.UpdateApiKeyRequest{
		ID: created.Key.ID, Name: "key-2", Role: "ROLE_ADMIN", State: "APPLICATION_KEY_STATE_RETIRED",
	})
	if err != nil || retired.State != "APPLICATION_KEY_STATE_RETIRED" || retired.ID != created.Key.ID {
		t.Fatalf("UpdateApiKey retire = %+v, %v", retired, err)
	}
	reactivated, err := c.UpdateApiKey(ctx, &client.UpdateApiKeyRequest{
		ID: created.Key.ID, Name: "key-2", Role: "ROLE_ADMIN", State: "APPLICATION_KEY_STATE_ACTIVE",
	})
	if err != nil || reactivated.State != "APPLICATION_KEY_STATE_ACTIVE" {
		t.Fatalf("UpdateApiKey reactivate = %+v, %v", reactivated, err)
	}
	if _, err := c.UpdateApiKey(ctx, &client.UpdateApiKeyRequest{
		ID: created.Key.ID, Name: "key-2", Role: "ROLE_ADMIN", State: "APPLICATION_KEY_STATE_DELETE",
	}); err == nil {
		t.Fatal("invalid API key state should fail")
	}
	all, err := c.ListAllApiKeys(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("ListAllApiKeys = %+v, %v", all, err)
	}

	if err := c.DeleteApiKey(ctx, created.Key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetApiKeyByID(ctx, created.Key.ID); !client.IsNotFound(err) {
		t.Fatalf("after delete: want not found, got %v", err)
	}
}

func TestClient_paginationWalksEveryPage(t *testing.T) {
	srv := fakereco.New(t)
	for i := range 5 {
		srv.SeedIntegration(client.Integration{Instance: client.AppInstance{ID: string(rune('a' + i))}, App: "GSUITE"})
	}
	c := client.New(srv.URL, fakereco.APIKey, "test").WithPageSize(2)

	all, err := c.ListAllIntegrations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("got %d integrations, want 5", len(all))
	}
	seen := map[string]bool{}
	for i := range all {
		seen[all[i].Instance.ID] = true
	}
	if len(seen) != 5 {
		t.Errorf("pages overlapped or skipped items: %v", seen)
	}
	if got := srv.CountRequests(http.MethodGet, "/integrations/list"); got != 3 {
		t.Errorf("made %d list requests, want 3 (pages of 2, 2, 1)", got)
	}
}

func TestClient_paginationOfEmptyList(t *testing.T) {
	c, srv := newFakeClient(t)
	all, err := c.ListAllPolicies(t.Context())
	if err != nil || len(all) != 0 {
		t.Fatalf("ListAllPolicies = %v, %v", all, err)
	}
	if got := srv.CountRequests(http.MethodGet, "/policies/list"); got != 1 {
		t.Errorf("made %d requests for an empty list, want 1", got)
	}
}

func TestClient_listAllUsersPaginatesAndFilters(t *testing.T) {
	srv := fakereco.New(t)
	for _, e := range []string{"a@x.com", "b@x.com", "c@x.com"} {
		srv.SeedUser(client.RecoUser{EmailAddress: e})
	}
	c := client.New(srv.URL, fakereco.APIKey, "test").WithPageSize(2)

	all, err := c.ListAllUsers(t.Context(), "")
	if err != nil || len(all) != 3 {
		t.Fatalf("ListAllUsers = %d users, %v", len(all), err)
	}
	one, err := c.ListAllUsers(t.Context(), `emailAddress eq "b@x.com"`)
	if err != nil || len(one) != 1 || one[0].EmailAddress != "b@x.com" {
		t.Fatalf("filtered ListAllUsers = %+v, %v", one, err)
	}
}

// flakyServer answers with the given statuses in order, then 200 with an empty roles list.
func flakyServer(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(hits.Add(1))
		if n <= len(statuses) {
			w.WriteHeader(statuses[n-1])
			_, _ = w.Write([]byte("boom"))
			return
		}
		_, _ = w.Write([]byte(`{"roles":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestClient_retriesGetOnTooManyRequestsAndServerErrors(t *testing.T) {
	srv, hits := flakyServer(t, http.StatusTooManyRequests, http.StatusServiceUnavailable)
	c := client.New(srv.URL, "k", "test", client.WithRetryBackoff(noBackoff))

	if _, err := c.ListAllRoles(t.Context()); err != nil {
		t.Fatalf("want success after retries, got %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("made %d requests, want 3", got)
	}
}

func TestClient_givesUpAfterMaxRetries(t *testing.T) {
	statuses := make([]int, client.MaxAttempts+2)
	for i := range statuses {
		statuses[i] = http.StatusInternalServerError
	}
	srv, hits := flakyServer(t, statuses...)
	c := client.New(srv.URL, "k", "test", client.WithRetryBackoff(noBackoff))

	_, err := c.ListAllRoles(t.Context())
	want := fmt.Sprintf("after %d attempts", client.MaxAttempts)
	if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want %q error carrying the body, got %v", want, err)
	}
	if got := int(hits.Load()); got != client.MaxAttempts {
		t.Errorf("made %d requests, want %d", got, client.MaxAttempts)
	}
}

func TestClient_rateLimitIsRiddenOutForLongerThanThreeAttempts(t *testing.T) {
	// A burst of consecutive 429s must still succeed within MaxAttempts.
	srv, hits := flakyServer(t, 429, 429, 429, 429, 429)
	c := client.New(srv.URL, "k", "test", client.WithRetryBackoff(noBackoff))

	if err := c.CreateRole(t.Context(), &client.RecoCustomRole{Name: "r"}); err != nil {
		t.Fatalf("want success once the rate limit clears, got %v", err)
	}
	if got := hits.Load(); got != 6 {
		t.Errorf("made %d requests, want 6", got)
	}
}

func TestClient_logsEveryRetryWait(t *testing.T) {
	var logs bytes.Buffer
	ctx := tflogtest.RootLogger(t.Context(), &logs)
	srv, _ := flakyServer(t, http.StatusTooManyRequests)
	c := client.New(srv.URL, "k", "test", client.WithRetryBackoff(noBackoff))

	if _, err := c.ListAllRoles(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := tflogtest.MultilineJSONDecode(&logs)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want one retry log line, got %d: %v", len(entries), entries)
	}
	got := entries[0]
	if got["@level"] != "warn" || got["wait_source"] != "backoff" || got["attempt"] != float64(1) ||
		got["max_attempts"] != float64(client.MaxAttempts) || !strings.Contains(fmt.Sprint(got["error"]), "HTTP 429") {
		t.Errorf("unexpected retry log entry: %v", got)
	}
}

func TestClient_pacesWritesAndReadsIndependently(t *testing.T) {
	srv := fakereco.New(t)
	const write, read = 150 * time.Millisecond, 40 * time.Millisecond
	c := client.New(srv.URL, fakereco.APIKey, "test", client.WithRetryBackoff(noBackoff), client.WithRequestPacing(read, write))
	ctx := t.Context()

	// Three writes need two full intervals between them.
	start := time.Now()
	for _, name := range []string{"a", "b", "c"} {
		if err := c.CreateRole(ctx, &client.RecoCustomRole{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 2*write {
		t.Errorf("3 writes took %v, want at least %v", elapsed, 2*write)
	}

	// Reads are paced on their own clock: they neither wait behind the writes nor slow them.
	start = time.Now()
	for range 3 {
		if _, err := c.ListAllRoles(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 2*read || elapsed > write+2*read+time.Second {
		t.Errorf("3 reads took %v, want about %v", elapsed, 2*read)
	}
}

func TestClient_pacingWaitHonorsContext(t *testing.T) {
	srv := fakereco.New(t)
	c := client.New(srv.URL, fakereco.APIKey, "test", client.WithRequestPacing(0, time.Hour))
	if err := c.CreateRole(t.Context(), &client.RecoCustomRole{Name: "first"}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := c.CreateRole(ctx, &client.RecoCustomRole{Name: "second"})
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "waiting") || !strings.Contains(err.Error(), "rate-limit slot") {
		t.Fatalf("want a context deadline error that says it happened while waiting for a write slot, got %v", err)
	}
	if n := srv.CountRequests(http.MethodPost, "/roles/create"); n != 1 {
		t.Errorf("the second write reached the API %d times in total, want only the first", n)
	}
}

func TestClient_logsLongPacingWaits(t *testing.T) {
	var logs bytes.Buffer
	ctx := tflogtest.RootLogger(t.Context(), &logs)
	srv := fakereco.New(t)
	c := client.New(srv.URL, fakereco.APIKey, "test", client.WithRequestPacing(0, 1100*time.Millisecond))

	for _, name := range []string{"first", "second"} {
		if err := c.CreateRole(ctx, &client.RecoCustomRole{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := tflogtest.MultilineJSONDecode(&logs)
	if err != nil {
		t.Fatal(err)
	}
	// The first write goes straight through; only the second waited, and for over a second.
	if len(entries) != 1 {
		t.Fatalf("want one pacing log line, got %d: %v", len(entries), entries)
	}
	got := entries[0]
	if got["@level"] != "info" || got["method"] != "POST" || got["path"] != "/roles/create" || got["wait"] == "" {
		t.Errorf("unexpected pacing log entry: %v", got)
	}
}

func TestClient_doesNotLogShortPacingWaits(t *testing.T) {
	var logs bytes.Buffer
	ctx := tflogtest.RootLogger(t.Context(), &logs)
	srv := fakereco.New(t)
	c := client.New(srv.URL, fakereco.APIKey, "test", client.WithRequestPacing(0, 50*time.Millisecond))

	for _, name := range []string{"a", "b", "c"} {
		if err := c.CreateRole(ctx, &client.RecoCustomRole{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	if logs.Len() != 0 {
		t.Errorf("waits under a second should not be logged, got %q", logs.String())
	}
}

func TestClient_retryBudgetErrorSaysWhatTheServerAskedFor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "100")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := client.New(srv.URL, "k", "test", client.WithRetryBudget(10*time.Second))
	_, err := c.ListAllRoles(t.Context())
	if err == nil || !strings.Contains(err.Error(), "server asked to wait 1m40s") {
		t.Fatalf("want an error quoting the server's Retry-After, got %v", err)
	}
}

func TestClient_honorsRetryAfter(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"roles":[]}`))
	}))
	defer srv.Close()

	// The configured backoff would wait an hour, so only the server's hint lets this finish.
	c := client.New(srv.URL, "k", "test", client.WithRetryBackoff(func(int) time.Duration { return time.Hour }))
	start := time.Now()
	if _, err := c.ListAllRoles(t.Context()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed > 10*time.Second {
		t.Errorf("waited %v, want about the 1s Retry-After", elapsed)
	}
}

func TestClient_stopsRetryingWhenTheRetryBudgetIsSpent(t *testing.T) {
	srv, hits := flakyServer(t, 429, 429, 429)
	// The backoff alone would outlast the budget, so the client must give up after the first attempt
	// instead of sleeping.
	c := client.New(srv.URL, "k", "test",
		client.WithRetryBackoff(func(int) time.Duration { return time.Hour }),
		client.WithRetryBudget(time.Minute))

	start := time.Now()
	_, err := c.ListAllRoles(t.Context())
	if err == nil || !strings.Contains(err.Error(), "retry time limit") || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("want a retry-time-limit error carrying the 429, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %v; should have failed fast", time.Since(start))
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("made %d requests, want 1", got)
	}
}

func TestClient_retryAfterBeyondTheBudgetFailsFast(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := client.New(srv.URL, "k", "test", client.WithRetryBudget(10*time.Second))
	start := time.Now()
	_, err := c.ListAllRoles(t.Context())
	if err == nil || !strings.Contains(err.Error(), "retry time limit") {
		t.Fatalf("want a retry-time-limit error, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %v; a Retry-After beyond the budget should fail immediately", time.Since(start))
	}
}

func TestClient_doesNotRetryPostOnServerError(t *testing.T) {
	srv, hits := flakyServer(t, http.StatusInternalServerError)
	c := client.New(srv.URL, "k", "test", client.WithRetryBackoff(noBackoff))

	err := c.CreateRole(t.Context(), &client.RecoCustomRole{Name: "r"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("want HTTP 500 error, got %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("POST was sent %d times, want exactly 1 (not idempotent)", got)
	}
}

func TestClient_doesNotRetryClientErrors(t *testing.T) {
	srv, hits := flakyServer(t, http.StatusBadRequest)
	c := client.New(srv.URL, "k", "test", client.WithRetryBackoff(noBackoff))

	_, err := c.ListAllRoles(t.Context())
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("want HTTP 400 error, got %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("made %d requests, want 1", got)
	}
}

func TestClient_notFoundIsTyped(t *testing.T) {
	srv, hits := flakyServer(t, http.StatusNotFound)
	c := client.New(srv.URL, "k", "test", client.WithRetryBackoff(noBackoff))

	err := c.DeleteRole(t.Context(), "gone")
	if !client.IsNotFound(err) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("made %d requests, want 1", got)
	}
}

func TestClient_malformedResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	_, err := client.New(srv.URL, "k", "test").ListAllRoles(t.Context())
	if err == nil || !strings.Contains(err.Error(), "unmarshal response") {
		t.Fatalf("want unmarshal error, got %v", err)
	}
}

func TestClient_contextCancelStopsRetries(t *testing.T) {
	srv, hits := flakyServer(t, 500, 500, 500)
	c := client.New(srv.URL, "k", "test",
		client.WithRetryBackoff(func(int) time.Duration { return time.Hour }),
		client.WithRetryBudget(48*time.Hour)) // large, so the context deadline is what stops the retries

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := c.ListAllRoles(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want context deadline error, got %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("made %d requests before the context expired, want 1", got)
	}
}
