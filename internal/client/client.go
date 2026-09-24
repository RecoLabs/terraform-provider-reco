package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	maxRetries     = 3
)

type Client struct {
	baseURL    string
	pageSize   int64
	httpClient *http.Client
}

func (c *Client) WithPageSize(n int64) *Client {
	c.pageSize = n
	return c
}

func (c *Client) effectivePageSize() int64 {
	if c.pageSize > 0 {
		return c.pageSize
	}
	return 1000
}

func New(baseURL, apiKey, version string) *Client {
	transport := &userAgentTransport{
		base:      http.DefaultTransport,
		userAgent: fmt.Sprintf("reco-tf-provider/%s", version),
		apiKey:    apiKey,
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   defaultTimeout,
			Transport: transport,
		},
	}
}

type userAgentTransport struct {
	base      http.RoundTripper
	userAgent string
	apiKey    string
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", t.userAgent)
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return t.base.RoundTrip(req)
}

func (c *Client) apiURL(path string) string {
	return c.baseURL + "/api/v1/external-api" + path
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}

		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.apiURL(path), reqBody)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// POST is not idempotent; the server may already have applied the request.
			if method == http.MethodPost {
				return err
			}
			lastErr = err
			continue
		}

		respBody, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 64<<20))
		_ = resp.Body.Close()
		if err != nil {
			if method == http.MethodPost {
				return fmt.Errorf("read response body: %w", err)
			}
			lastErr = fmt.Errorf("read response body: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests ||
			(resp.StatusCode >= 500 && method != http.MethodPost) {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			return &NotFoundError{Message: fmt.Sprintf("HTTP 404: %s", string(respBody))}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		}

		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("unmarshal response: %w", err)
			}
		}
		return nil
	}
	return fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string { return e.Message }

func IsNotFound(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

func scimQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func (c *Client) CreateUser(ctx context.Context, user *RecoUser) (*RecoUser, error) {
	var created RecoUser
	if err := c.do(ctx, http.MethodPost, "/reco-users/create", CreateRecoUserRequest{User: *user}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (c *Client) GetUserByID(ctx context.Context, userID string) (*RecoUser, error) {
	filter := "userId eq " + scimQuote(userID)
	users, err := c.listUsers(ctx, filter, 0, 2)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, &NotFoundError{Message: fmt.Sprintf("user with userId %q not found", userID)}
	}
	return &users[0], nil
}

func (c *Client) GetUserByEmail(ctx context.Context, email string) (*RecoUser, error) {
	filter := "emailAddress eq " + scimQuote(email)
	users, err := c.listUsers(ctx, filter, 0, 2)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, &NotFoundError{Message: fmt.Sprintf("user with email %q not found", email)}
	}
	if len(users) > 1 {
		return nil, fmt.Errorf("ambiguous: %d users found for email %q", len(users), email)
	}
	return &users[0], nil
}

func (c *Client) UpdateUser(ctx context.Context, req *UpdateRecoUserRequest) error {
	return c.do(ctx, http.MethodPut, "/reco-users/update", req, nil)
}

func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	return c.do(ctx, http.MethodDelete, "/reco-users/"+url.PathEscape(userID), nil, nil)
}

func (c *Client) listUsers(ctx context.Context, filter string, startIndex, count int64) ([]RecoUser, error) {
	path := "/reco-users/list"
	params := url.Values{}
	if filter != "" {
		params.Set("filters", filter)
	}
	if count > 0 {
		params.Set("count", fmt.Sprintf("%d", count))
	}
	if startIndex > 0 {
		params.Set("startIndex", fmt.Sprintf("%d", startIndex))
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp listRecoUsersResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Users, nil
}

func (c *Client) ListAllUsers(ctx context.Context, filter string) ([]RecoUser, error) {
	basePath := "/reco-users/list"
	if filter != "" {
		params := url.Values{}
		params.Set("filters", filter)
		basePath += "?" + params.Encode()
	}
	return paginateList[RecoUser](ctx, c, basePath, func(r json.RawMessage) ([]RecoUser, int64, error) {
		var resp listRecoUsersResponse
		if err := json.Unmarshal(r, &resp); err != nil {
			return nil, 0, err
		}
		return resp.Users, int64(resp.Total), nil
	})
}

func (c *Client) CreateRole(ctx context.Context, role *RecoCustomRole) error {
	return c.do(ctx, http.MethodPost, "/roles/create", CreateRecoCustomRoleRequest{Role: *role}, nil)
}

func (c *Client) GetRoleByName(ctx context.Context, name string) (*RecoRole, error) {
	filter := "name eq " + scimQuote(name)
	params := url.Values{}
	params.Set("filters", filter)
	path := "/roles/list?" + params.Encode()
	var resp listRecoRolesResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if len(resp.Roles) == 0 {
		return nil, &NotFoundError{Message: fmt.Sprintf("role with name %q not found", name)}
	}
	if len(resp.Roles) > 1 {
		return nil, fmt.Errorf("ambiguous: %d roles found for name %q", len(resp.Roles), name)
	}
	return &resp.Roles[0], nil
}

func (c *Client) UpdateRole(ctx context.Context, role *RecoCustomRole, originalName string) error {
	req := UpdateRecoCustomRoleRequest{Role: *role, OriginalName: originalName}
	return c.do(ctx, http.MethodPut, "/roles/update", req, nil)
}

func (c *Client) DeleteRole(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/roles/"+url.PathEscape(name), nil, nil)
}

func (c *Client) ListAllRoles(ctx context.Context) ([]RecoRole, error) {
	var resp listRecoRolesResponse
	if err := c.do(ctx, http.MethodGet, "/roles/list", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Roles, nil
}

func (c *Client) CreatePostureCheck(ctx context.Context, def *PostureCheckDefinition) (string, error) {
	var resp CreatePostureCheckResponse
	if err := c.do(ctx, http.MethodPost, "/posture-checks/create", CreatePostureCheckRequest{Definition: *def}, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *Client) GetPostureCheckByID(ctx context.Context, id string) (*PostureCheck, error) {
	filter := "id eq " + scimQuote(id)
	params := url.Values{}
	params.Set("filters", filter)
	path := "/posture-checks/list?" + params.Encode()
	var resp listPostureChecksResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if len(resp.PostureChecks) == 0 {
		return nil, &NotFoundError{Message: fmt.Sprintf("posture check with id %q not found", id)}
	}
	return &resp.PostureChecks[0], nil
}

func (c *Client) UpdatePostureCheck(ctx context.Context, id string, def *PostureCheckDefinition) error {
	return c.do(ctx, http.MethodPut, "/posture-checks/update", UpdatePostureCheckRequest{ID: id, Definition: *def}, nil)
}

func (c *Client) DeletePostureCheck(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/posture-checks/"+url.PathEscape(id), nil, nil)
}

func (c *Client) ListAllPostureChecks(ctx context.Context) ([]PostureCheck, error) {
	return paginateList[PostureCheck](ctx, c, "/posture-checks/list", func(r json.RawMessage) ([]PostureCheck, int64, error) {
		var resp listPostureChecksResponse
		if err := json.Unmarshal(r, &resp); err != nil {
			return nil, 0, err
		}
		total, err := resp.TotalResults.Int64()
		if err != nil {
			return nil, 0, fmt.Errorf("parse totalResults %q: %w", resp.TotalResults, err)
		}
		return resp.PostureChecks, total, nil
	})
}

func (c *Client) CreateThreatDetectionPolicy(ctx context.Context, def *ThreatDetectionPolicyDefinition) (string, error) {
	var resp CreateThreatDetectionPolicyResponse
	if err := c.do(ctx, http.MethodPost, "/policies/create", CreateThreatDetectionPolicyRequest{Definition: *def}, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *Client) GetThreatDetectionPolicyByID(ctx context.Context, id string) (*ThreatDetectionPolicy, error) {
	filter := "id eq " + scimQuote(id)
	params := url.Values{}
	params.Set("filters", filter)
	path := "/policies/list?" + params.Encode()
	var resp listThreatDetectionPoliciesResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if len(resp.Policies) == 0 {
		return nil, &NotFoundError{Message: fmt.Sprintf("policy with id %q not found", id)}
	}
	return &resp.Policies[0], nil
}

func (c *Client) UpdateThreatDetectionPolicy(ctx context.Context, id string, def *ThreatDetectionPolicyDefinition) error {
	return c.do(ctx, http.MethodPut, "/policies/update", UpdateThreatDetectionPolicyRequest{ID: id, Definition: *def}, nil)
}

func (c *Client) DeleteThreatDetectionPolicy(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/policies/"+url.PathEscape(id), nil, nil)
}

func (c *Client) CreateApiKey(ctx context.Context, req CreateApiKeyRequest) (*CreateApiKeyResponse, error) {
	var resp CreateApiKeyResponse
	if err := c.do(ctx, http.MethodPost, "/api-keys/create", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetApiKeyByID(ctx context.Context, id string) (*ApiKey, error) {
	var resp listApiKeysResponse
	if err := c.do(ctx, http.MethodGet, "/api-keys/list", nil, &resp); err != nil {
		return nil, err
	}
	for i := range resp.Keys {
		if resp.Keys[i].ID == id {
			return &resp.Keys[i], nil
		}
	}
	return nil, &NotFoundError{Message: fmt.Sprintf("API key with id %q not found", id)}
}

func (c *Client) UpdateApiKey(ctx context.Context, req UpdateApiKeyRequest) (*ApiKey, error) {
	var resp UpdateApiKeyResponse
	if err := c.do(ctx, http.MethodPut, "/api-keys/update", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Key, nil
}

func (c *Client) DeleteApiKey(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api-keys/"+url.PathEscape(id), nil, nil)
}

func (c *Client) ListAllApiKeys(ctx context.Context) ([]ApiKey, error) {
	var resp listApiKeysResponse
	if err := c.do(ctx, http.MethodGet, "/api-keys/list", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

func (c *Client) ListAllPolicies(ctx context.Context) ([]ThreatDetectionPolicy, error) {
	return paginateList[ThreatDetectionPolicy](ctx, c, "/policies/list", func(r json.RawMessage) ([]ThreatDetectionPolicy, int64, error) {
		var resp listThreatDetectionPoliciesResponse
		if err := json.Unmarshal(r, &resp); err != nil {
			return nil, 0, err
		}
		total, err := resp.TotalResults.Int64()
		if err != nil {
			return nil, 0, fmt.Errorf("parse totalResults %q: %w", resp.TotalResults, err)
		}
		return resp.Policies, total, nil
	})
}

func (c *Client) ListAllIntegrations(ctx context.Context) ([]Integration, error) {
	return paginateList[Integration](ctx, c, "/integrations/list", func(r json.RawMessage) ([]Integration, int64, error) {
		var resp listIntegrationsResponse
		if err := json.Unmarshal(r, &resp); err != nil {
			return nil, 0, err
		}
		total, err := resp.TotalResults.Int64()
		if err != nil {
			return nil, 0, fmt.Errorf("parse totalResults %q: %w", resp.TotalResults, err)
		}
		return resp.Integrations, total, nil
	})
}

func paginateList[T any](
	ctx context.Context,
	c *Client,
	basePath string,
	unmarshal func(json.RawMessage) ([]T, int64, error),
) ([]T, error) {
	pageSize := c.effectivePageSize()
	var all []T
	startIndex := int64(1)

	base, err := url.Parse(basePath)
	if err != nil {
		return nil, fmt.Errorf("paginateList: invalid basePath %q: %w", basePath, err)
	}

	for {
		params := base.Query()
		params.Set("count", fmt.Sprintf("%d", pageSize))
		params.Set("startIndex", fmt.Sprintf("%d", startIndex))
		base.RawQuery = params.Encode()

		var raw json.RawMessage
		if err := c.do(ctx, http.MethodGet, base.String(), nil, &raw); err != nil {
			return nil, err
		}

		items, total, err := unmarshal(raw)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)

		if int64(len(all)) >= total || len(items) == 0 {
			break
		}
		startIndex += int64(len(items))
	}
	return all, nil
}
