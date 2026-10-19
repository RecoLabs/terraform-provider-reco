// Package fakereco is an in-memory fake of the Reco external API, used by unit tests so that the
// client, resources and data sources can be exercised without a real tenant.
//
// It mirrors the details the provider depends on: the /api/v1/external-api prefix, bearer auth,
// SCIM-style `field eq "value"` filters, 1-based startIndex pagination and the per-endpoint
// response envelopes (users report `total`, the other lists report `totalResults`).
package fakereco

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/recolabs/terraform-provider-reco/internal/client"
)

const (
	// APIKey is the bearer token the fake accepts.
	APIKey = "fake-api-key"

	apiPrefix = "/api/v1/external-api"
	epoch     = "2026-01-01T00:00:00Z"

	noExpiration = "1970-01-01T00:00:00Z"
)

var filterRe = regexp.MustCompile(`^(\w+) eq "((?:[^"\\]|\\.)*)"$`)

type failRule struct {
	method, pathContains string
	status               int
	body                 string
}

// Server is a running fake Reco API.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	seq      int
	users    []client.RecoUser
	roles    []client.RecoRole
	posture  []client.PostureCheck
	policies []client.ThreatDetectionPolicy
	keys     []client.ApiKey
	integs   []client.Integration
	fails    []failRule
	requests []string
}

// New starts a fake API and shuts it down when the test ends.
func New(tb testing.TB) *Server {
	tb.Helper()
	s := Start()
	tb.Cleanup(s.Close)
	return s
}

// Start starts a fake API that the caller must Close. It is seeded with the two built-in system
// roles that real tenants always have.
func Start() *Server {
	s := &Server{
		roles: []client.RecoRole{
			{Name: "ROLE_SECURITY_VIEWER", Description: "Read-only security viewer", Type: "USER_ROLE_TYPE_SYSTEM", Permissions: []string{"PERM_ALERTS_READ"}},
			{Name: "ROLE_ADMIN", Description: "Administrator", Type: "USER_ROLE_TYPE_SYSTEM", Permissions: []string{"PERM_ALERTS_READ", "PERM_ADMIN"}},
		},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// FailWith makes every request with the given method whose path contains pathContains fail with
// the given status. Use 4xx statuses: the client retries 429/5xx, which slows tests down.
func (s *Server) FailWith(method, pathContains string, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails = append(s.fails, failRule{method, pathContains, status, body})
}

// ClearFailures removes all rules added by FailWith.
func (s *Server) ClearFailures() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails = nil
}

// Requests returns "METHOD path" for every request served so far, excluding the query string.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

// CountRequests counts requests served so far for the given method and path (without the API prefix).
func (s *Server) CountRequests(method, path string) int {
	want := method + " " + apiPrefix + path
	n := 0
	for _, r := range s.Requests() {
		if r == want {
			n++
		}
	}
	return n
}

// SeedUser adds a user and returns it with its generated ID.
func (s *Server) SeedUser(u client.RecoUser) client.RecoUser {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addUser(u)
}

// SeedRole adds a custom role.
func (s *Server) SeedRole(r client.RecoRole) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Type == "" {
		r.Type = "USER_ROLE_TYPE_CUSTOM"
	}
	s.roles = append(s.roles, r)
}

// SeedIntegration adds an integration; the API has no endpoint to create them.
func (s *Server) SeedIntegration(i client.Integration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.integs = append(s.integs, i)
}

// SeedPolicy adds a threat detection policy and returns its generated ID.
func (s *Server) SeedPolicy(p client.ThreatDetectionPolicy) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.ID = s.nextID("policy")
	s.policies = append(s.policies, p)
	return p.ID
}

// SeedPostureCheck adds a posture check and returns its generated ID.
func (s *Server) SeedPostureCheck(c client.PostureCheck) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = s.nextID("posture")
	s.posture = append(s.posture, c)
	return c.ID
}

// SeedAPIKey adds an API key and returns its generated ID.
func (s *Server) SeedAPIKey(k client.ApiKey) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	k.ID = s.nextID("key")
	s.keys = append(s.keys, k)
	return k.ID
}

// DeleteRole removes a role behind the provider's back, to simulate out-of-band drift.
func (s *Server) DeleteRole(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return remove(&s.roles, func(r client.RecoRole) bool { return r.Name == name })
}

// DeleteUser removes a user behind the provider's back.
func (s *Server) DeleteUser(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return remove(&s.users, func(u client.RecoUser) bool { return u.UserID == id })
}

// DeletePostureCheck removes a posture check behind the provider's back.
func (s *Server) DeletePostureCheck(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return remove(&s.posture, func(c client.PostureCheck) bool { return c.ID == id })
}

// DeletePolicy removes a threat detection policy behind the provider's back.
func (s *Server) DeletePolicy(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return remove(&s.policies, func(p client.ThreatDetectionPolicy) bool { return p.ID == id })
}

// DeleteAPIKey removes an API key behind the provider's back.
func (s *Server) DeleteAPIKey(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return remove(&s.keys, func(k client.ApiKey) bool { return k.ID == id })
}

// Users returns a copy of the current users.
func (s *Server) Users() []client.RecoUser {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]client.RecoUser(nil), s.users...)
}

// Roles returns a copy of the current roles, system roles included.
func (s *Server) Roles() []client.RecoRole {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]client.RecoRole(nil), s.roles...)
}

// PostureChecks returns a copy of the current posture checks.
func (s *Server) PostureChecks() []client.PostureCheck {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]client.PostureCheck(nil), s.posture...)
}

// Policies returns a copy of the current threat detection policies.
func (s *Server) Policies() []client.ThreatDetectionPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]client.ThreatDetectionPolicy(nil), s.policies...)
}

// APIKeys returns a copy of the current API keys.
func (s *Server) APIKeys() []client.ApiKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]client.ApiKey(nil), s.keys...)
}

func remove[T any](list *[]T, match func(T) bool) bool {
	for i := range *list {
		if match((*list)[i]) {
			*list = append((*list)[:i], (*list)[i+1:]...)
			return true
		}
	}
	return false
}

func (s *Server) nextID(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s-%d", prefix, s.seq)
}

func (s *Server) addUser(u client.RecoUser) client.RecoUser {
	u.UserID = s.nextID("user")
	if u.EnablementStatus == "" {
		u.EnablementStatus = "enabled"
	}
	if u.Segments == nil {
		u.Segments = []string{}
	}
	if u.AuthMethods == nil {
		u.AuthMethods = []string{"password"}
	}
	created := epoch
	u.CreationTime = &created
	s.users = append(s.users, u)
	return u
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = append(s.requests, r.Method+" "+r.URL.Path)

	if r.Header.Get("Authorization") != "Bearer "+APIKey {
		writeError(w, http.StatusUnauthorized, "invalid or missing bearer token")
		return
	}
	if !strings.HasPrefix(r.URL.Path, apiPrefix+"/") {
		writeError(w, http.StatusNotFound, "unknown path "+r.URL.Path)
		return
	}
	for _, f := range s.fails {
		if f.method == r.Method && strings.Contains(r.URL.Path, f.pathContains) {
			writeError(w, f.status, f.body)
			return
		}
	}

	resource, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, apiPrefix+"/"), "/")
	switch resource {
	case "reco-users":
		s.serveUsers(w, r, rest)
	case "roles":
		s.serveRoles(w, r, rest)
	case "posture-checks":
		s.servePostureChecks(w, r, rest)
	case "policies":
		s.servePolicies(w, r, rest)
	case "api-keys":
		s.serveAPIKeys(w, r, rest)
	case "integrations":
		s.serveIntegrations(w, r, rest)
	default:
		writeError(w, http.StatusNotFound, "unknown resource "+resource)
	}
}

func (s *Server) serveUsers(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case r.Method == http.MethodPost && rest == "create":
		var req client.CreateRecoUserRequest
		if !readJSON(w, r, &req) {
			return
		}
		for i := range s.users {
			if s.users[i].EmailAddress == req.User.EmailAddress {
				writeError(w, http.StatusConflict, "user with this email already exists")
				return
			}
		}
		writeJSON(w, s.addUser(req.User))
	case r.Method == http.MethodGet && rest == "list":
		page, total, ok := filterAndPage(w, r, s.users, func(u client.RecoUser, field string) string {
			switch field {
			case "userId":
				return u.UserID
			case "emailAddress":
				return u.EmailAddress
			case "name":
				return u.Name
			}
			return ""
		})
		if ok {
			writeJSON(w, map[string]any{"users": nonNil(page), "total": total})
		}
	case r.Method == http.MethodPut && rest == "update":
		var req client.UpdateRecoUserRequest
		if !readJSON(w, r, &req) {
			return
		}
		for i := range s.users {
			if s.users[i].UserID != req.User.UserID {
				continue
			}
			u := req.User
			u.CreationTime = s.users[i].CreationTime
			u.AuthMethods = s.users[i].AuthMethods
			u.EnablementStatus = s.users[i].EnablementStatus
			if req.EnablementStatus != "" {
				u.EnablementStatus = req.EnablementStatus
			}
			s.users[i] = u
			writeJSON(w, struct{}{})
			return
		}
		writeError(w, http.StatusNotFound, "user not found")
	case r.Method == http.MethodDelete:
		if !remove(&s.users, func(u client.RecoUser) bool { return u.UserID == rest }) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, struct{}{})
	default:
		writeError(w, http.StatusNotFound, "no such users endpoint")
	}
}

func (s *Server) serveRoles(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case r.Method == http.MethodPost && rest == "create":
		var req client.CreateRecoCustomRoleRequest
		if !readJSON(w, r, &req) {
			return
		}
		for i := range s.roles {
			if s.roles[i].Name == req.Role.Name {
				writeError(w, http.StatusConflict, "role already exists")
				return
			}
		}
		created := epoch
		s.roles = append(s.roles, customToRole(req.Role, "USER_ROLE_TYPE_CUSTOM", &created))
		writeJSON(w, struct{}{})
	case r.Method == http.MethodGet && rest == "list":
		matches, ok := filterOnly(w, r, s.roles, func(role client.RecoRole, field string) string {
			if field == "name" {
				return role.Name
			}
			return ""
		})
		if ok {
			writeJSON(w, map[string]any{"roles": nonNil(matches)})
		}
	case r.Method == http.MethodPut && rest == "update":
		var req client.UpdateRecoCustomRoleRequest
		if !readJSON(w, r, &req) {
			return
		}
		for i := range s.roles {
			if s.roles[i].Name == req.OriginalName && s.roles[i].Type == "USER_ROLE_TYPE_CUSTOM" {
				s.roles[i] = customToRole(req.Role, s.roles[i].Type, s.roles[i].CreatedAt)
				writeJSON(w, struct{}{})
				return
			}
		}
		writeError(w, http.StatusNotFound, "role not found")
	case r.Method == http.MethodDelete:
		if !remove(&s.roles, func(role client.RecoRole) bool { return role.Name == rest && role.Type == "USER_ROLE_TYPE_CUSTOM" }) {
			writeError(w, http.StatusNotFound, "role not found")
			return
		}
		writeJSON(w, struct{}{})
	default:
		writeError(w, http.StatusNotFound, "no such roles endpoint")
	}
}

func customToRole(c client.RecoCustomRole, roleType string, createdAt *string) client.RecoRole {
	return client.RecoRole{
		Name:        c.Name,
		Description: c.Description,
		Permissions: c.Permissions,
		Resources:   c.Resources,
		Type:        roleType,
		CreatedAt:   createdAt,
	}
}

func (s *Server) servePostureChecks(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case r.Method == http.MethodPost && rest == "create":
		var req client.CreatePostureCheckRequest
		if !readJSON(w, r, &req) {
			return
		}
		c := postureFromDef(req.Definition)
		c.ID = s.nextID("posture")
		s.posture = append(s.posture, c)
		writeJSON(w, client.CreatePostureCheckResponse{ID: c.ID})
	case r.Method == http.MethodGet && rest == "list":
		page, total, ok := filterAndPage(w, r, s.posture, func(c client.PostureCheck, field string) string {
			switch field {
			case "id":
				return c.ID
			case "name":
				return c.Name
			}
			return ""
		})
		if ok {
			writeJSON(w, map[string]any{"totalResults": total, "itemsPerPage": len(page), "postureChecks": nonNil(page)})
		}
	case r.Method == http.MethodPut && rest == "update":
		var req client.UpdatePostureCheckRequest
		if !readJSON(w, r, &req) {
			return
		}
		for i := range s.posture {
			if s.posture[i].ID == req.ID {
				c := postureFromDef(req.Definition)
				c.ID = req.ID
				s.posture[i] = c
				writeJSON(w, struct{}{})
				return
			}
		}
		writeError(w, http.StatusNotFound, "posture check not found")
	case r.Method == http.MethodDelete:
		if !remove(&s.posture, func(c client.PostureCheck) bool { return c.ID == rest }) {
			writeError(w, http.StatusNotFound, "posture check not found")
			return
		}
		writeJSON(w, struct{}{})
	default:
		writeError(w, http.StatusNotFound, "no such posture-checks endpoint")
	}
}

func postureFromDef(d client.PostureCheckDefinition) client.PostureCheck {
	return client.PostureCheck{
		Name:        d.Title,
		Description: d.Description,
		Tags:        d.Tags,
		Severity:    d.ViolationRiskLevel,
		AppSource:   d.DataSource,
		PolicyType:  "POLICY_TYPE_POSTURE",
	}
}

func (s *Server) servePolicies(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case r.Method == http.MethodPost && rest == "create":
		var req client.CreateThreatDetectionPolicyRequest
		if !readJSON(w, r, &req) {
			return
		}
		p := policyFromDef(req.Definition)
		p.ID = s.nextID("policy")
		s.policies = append(s.policies, p)
		writeJSON(w, client.CreateThreatDetectionPolicyResponse{ID: p.ID})
	case r.Method == http.MethodGet && rest == "list":
		page, total, ok := filterAndPage(w, r, s.policies, func(p client.ThreatDetectionPolicy, field string) string {
			switch field {
			case "id":
				return p.ID
			case "name":
				return p.Name
			}
			return ""
		})
		if ok {
			writeJSON(w, map[string]any{"totalResults": total, "itemsPerPage": len(page), "policies": nonNil(page)})
		}
	case r.Method == http.MethodPut && rest == "update":
		var req client.UpdateThreatDetectionPolicyRequest
		if !readJSON(w, r, &req) {
			return
		}
		for i := range s.policies {
			if s.policies[i].ID == req.ID {
				p := policyFromDef(req.Definition)
				p.ID = req.ID
				s.policies[i] = p
				writeJSON(w, struct{}{})
				return
			}
		}
		writeError(w, http.StatusNotFound, "policy not found")
	case r.Method == http.MethodDelete:
		if !remove(&s.policies, func(p client.ThreatDetectionPolicy) bool { return p.ID == rest }) {
			writeError(w, http.StatusNotFound, "policy not found")
			return
		}
		writeJSON(w, struct{}{})
	default:
		writeError(w, http.StatusNotFound, "no such policies endpoint")
	}
}

func policyFromDef(d client.ThreatDetectionPolicyDefinition) client.ThreatDetectionPolicy {
	created := epoch
	status := d.DefaultStatus
	if status == "" {
		status = "POLICY_STATUS_OFF"
	}
	return client.ThreatDetectionPolicy{
		Name:       d.Title,
		Apps:       []string{d.DataSource},
		Severity:   d.ViolationRiskLevel,
		Status:     status,
		Tags:       d.Tags,
		CreatedAt:  &created,
		PolicyType: "POLICY_TYPE_THREAT_DETECTION",
	}
}

func (s *Server) serveAPIKeys(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case r.Method == http.MethodPost && rest == "create":
		var req client.CreateApiKeyRequest
		if !readJSON(w, r, &req) {
			return
		}
		for i := range s.keys {
			if s.keys[i].Name == req.Name {
				writeError(w, http.StatusConflict, "an API key with this name already exists")
				return
			}
		}
		created := time.Now().UTC().Format(time.RFC3339)
		expiration := req.Expiration
		if expiration == nil {
			// The real API reports "no expiration" as the Unix epoch.
			noExpiry := noExpiration
			expiration = &noExpiry
		}
		key := client.ApiKey{
			ID:           s.nextID("key"),
			Name:         req.Name,
			Role:         req.Role,
			CreatedBy:    "tf-test@example.com",
			State:        "APPLICATION_KEY_STATE_ACTIVE",
			PermittedIps: req.PermittedIps,
			Expiration:   expiration,
			CreatedAt:    &created,
		}
		s.keys = append(s.keys, key)
		writeJSON(w, client.CreateApiKeyResponse{Key: key, Secret: "secret-for-" + key.ID})
	case r.Method == http.MethodGet && rest == "list":
		writeJSON(w, map[string]any{"keys": nonNil(s.keys)})
	case r.Method == http.MethodPut && rest == "update":
		var req client.UpdateApiKeyRequest
		if !readJSON(w, r, &req) {
			return
		}
		for i := range s.keys {
			if s.keys[i].ID == req.ID {
				s.keys[i].Name = req.Name
				s.keys[i].Role = req.Role
				s.keys[i].PermittedIps = req.PermittedIps
				writeJSON(w, client.UpdateApiKeyResponse{Key: s.keys[i]})
				return
			}
		}
		writeError(w, http.StatusNotFound, "API key not found")
	case r.Method == http.MethodDelete:
		if !remove(&s.keys, func(k client.ApiKey) bool { return k.ID == rest }) {
			writeError(w, http.StatusNotFound, "API key not found")
			return
		}
		writeJSON(w, struct{}{})
	default:
		writeError(w, http.StatusNotFound, "no such api-keys endpoint")
	}
}

func (s *Server) serveIntegrations(w http.ResponseWriter, r *http.Request, rest string) {
	if r.Method != http.MethodGet || rest != "list" {
		writeError(w, http.StatusNotFound, "no such integrations endpoint")
		return
	}
	page, total, ok := filterAndPage(w, r, s.integs, func(i client.Integration, field string) string {
		if field == "app" {
			return i.App
		}
		return ""
	})
	if ok {
		writeJSON(w, map[string]any{"totalResults": total, "itemsPerPage": len(page), "integrations": nonNil(page)})
	}
}

// filterOnly applies the optional `filters` query parameter and returns every match.
func filterOnly[T any](w http.ResponseWriter, r *http.Request, items []T, field func(T, string) string) ([]T, bool) {
	filter := r.URL.Query().Get("filters")
	if filter == "" {
		return items, true
	}
	m := filterRe.FindStringSubmatch(filter)
	if m == nil {
		writeError(w, http.StatusBadRequest, "unsupported filter: "+filter)
		return nil, false
	}
	want := strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(m[2])
	var out []T
	for _, it := range items {
		if field(it, m[1]) == want {
			out = append(out, it)
		}
	}
	return out, true
}

// filterAndPage applies `filters`, then `startIndex` (1-based; absent or 0 means the start) and
// `count` (absent means everything). It returns the page and the total number of matches.
func filterAndPage[T any](w http.ResponseWriter, r *http.Request, items []T, field func(T, string) string) ([]T, int, bool) {
	matches, ok := filterOnly(w, r, items, field)
	if !ok {
		return nil, 0, false
	}
	q := r.URL.Query()
	start := 0
	if v := q.Get("startIndex"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid startIndex")
			return nil, 0, false
		}
		start = max(n-1, 0)
	}
	start = min(start, len(matches))
	end := len(matches)
	if v := q.Get("count"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid count")
			return nil, 0, false
		}
		end = min(start+n, len(matches))
	}
	return matches[start:end], len(matches), true
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if err := json.Unmarshal(body, v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
