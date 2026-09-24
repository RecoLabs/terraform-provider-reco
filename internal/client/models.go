package client

import "encoding/json"

type RecoUser struct {
	UserID                 string          `json:"userId,omitempty"`
	EmailAddress           string          `json:"emailAddress,omitempty"`
	Name                   string          `json:"name,omitempty"`
	UserRoles              []string        `json:"userRoles"`
	Segments               []string        `json:"segments"`
	Expiration             *string         `json:"expiration,omitempty"`
	IsBypassSSOEnforcement bool            `json:"isBypassSsoEnforcement"`
	AuthMethods            []string        `json:"authMethods,omitempty"`
	EnablementStatus       string          `json:"enablementStatus,omitempty"`
	ProfilePictureURL      string          `json:"profilePictureUrl,omitempty"`
	CreationTime           *string         `json:"creationTime,omitempty"`
	LastLoginTime          *string         `json:"lastLoginTime,omitempty"`
	OauthConnections       map[string]bool `json:"oauthConnections,omitempty"`
}

type CreateRecoUserRequest struct {
	User RecoUser `json:"user"`
}

type UpdateRecoUserRequest struct {
	User             RecoUser `json:"user"`
	EnablementStatus string   `json:"enablementStatus,omitempty"`
}

type listRecoUsersResponse struct {
	Users []RecoUser `json:"users"`
	Total int32      `json:"total"`
}

type ResourcePermission struct {
	ResourceName string `json:"resourceName"`
	Resource     string `json:"resource"`
	Permission   string `json:"permission"`
}

type RecoCustomRole struct {
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Permissions []string             `json:"permissions"`
	Resources   []ResourcePermission `json:"resources"`
}

type RecoRole struct {
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Permissions []string             `json:"permissions,omitempty"`
	Type        string               `json:"type,omitempty"`
	CreatedAt   *string              `json:"createdAt,omitempty"`
	Resources   []ResourcePermission `json:"resources,omitempty"`
}

type CreateRecoCustomRoleRequest struct {
	Role RecoCustomRole `json:"role"`
}

type UpdateRecoCustomRoleRequest struct {
	Role         RecoCustomRole `json:"role"`
	OriginalName string         `json:"originalName,omitempty"`
}

type listRecoRolesResponse struct {
	Roles []RecoRole `json:"roles"`
}

type ThreatDetectionPolicy struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Apps               []string `json:"apps,omitempty"`
	DisplaySource      string   `json:"displaySource,omitempty"`
	RiskType           string   `json:"riskType,omitempty"`
	Severity           string   `json:"severity,omitempty"`
	Type               string   `json:"type,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	Status             string   `json:"status,omitempty"`
	CreatedAt          *string  `json:"createdAt,omitempty"`
	PolicyType         string   `json:"policyType,omitempty"`
	OpenAlerts         int32    `json:"openAlerts"`
	AllAlerts          int32    `json:"allAlerts"`
	IsTemporary        bool     `json:"isTemporary"`
	ActivityPolicyType string   `json:"activityPolicyType,omitempty"`
}

type listThreatDetectionPoliciesResponse struct {
	TotalResults json.Number             `json:"totalResults"`
	ItemsPerPage json.Number             `json:"itemsPerPage"`
	Policies     []ThreatDetectionPolicy `json:"policies"`
}

type AppInstance struct {
	ID      string `json:"id"`
	AppName string `json:"appName,omitempty"`
}

type Integration struct {
	Instance        AppInstance `json:"instance"`
	App             string      `json:"app,omitempty"`
	Status          string      `json:"status,omitempty"`
	HasNewEndpoints bool        `json:"hasNewEndpoints"`
	CreatedOn       *string     `json:"createdOn,omitempty"`
	LastUpdatedOn   *string     `json:"lastUpdatedOn,omitempty"`
	LastUpdatedBy   string      `json:"lastUpdatedBy,omitempty"`
	SeverityConfig  string      `json:"severityConfig,omitempty"`
	ErrorDetails    string      `json:"errorDetails,omitempty"`
}

type listIntegrationsResponse struct {
	TotalResults json.Number   `json:"totalResults"`
	ItemsPerPage json.Number   `json:"itemsPerPage"`
	Integrations []Integration `json:"integrations"`
}

type PostureCheckDefinition struct {
	Title                string   `json:"title"`
	Description          string   `json:"description,omitempty"`
	DataSource           string   `json:"dataSource"`
	ViolationRiskLevel   string   `json:"violationRiskLevel"`
	ViolationRiskType    string   `json:"violationRiskType"`
	DefaultStatus        string   `json:"defaultStatus,omitempty"`
	PostureUniqueJsonata string   `json:"postureUniqueJsonata"`
	PostureValueJsonata  string   `json:"postureValueJsonata,omitempty"`
	ConditionsJsonata    string   `json:"conditionsJsonata"`
	StatusJsonata        string   `json:"statusJsonata,omitempty"`
	HowToRemediate       string   `json:"howToRemediate,omitempty"`
	WhyShouldICare       string   `json:"whyShouldICare,omitempty"`
	Tags                 []string `json:"tags,omitempty"`
}

type CreatePostureCheckRequest struct {
	Definition PostureCheckDefinition `json:"definition"`
}

type CreatePostureCheckResponse struct {
	ID string `json:"id"`
}

type UpdatePostureCheckRequest struct {
	ID         string                 `json:"id"`
	Definition PostureCheckDefinition `json:"definition"`
}

type PostureCheck struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Apps        []string `json:"apps,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Severity    string   `json:"severity,omitempty"`
	AppSource   string   `json:"appSource,omitempty"`
	PolicyType  string   `json:"policyType,omitempty"`
}

type listPostureChecksResponse struct {
	TotalResults  json.Number    `json:"totalResults"`
	ItemsPerPage  json.Number    `json:"itemsPerPage"`
	PostureChecks []PostureCheck `json:"postureChecks"`
}

type ThreatDetectionPolicyDefinition struct {
	Title               string   `json:"title"`
	Description         string   `json:"description,omitempty"`
	DataSource          string   `json:"dataSource"`
	ViolationRiskLevel  string   `json:"violationRiskLevel"`
	ViolationRiskType   string   `json:"violationRiskType"`
	DefaultStatus       string   `json:"defaultStatus,omitempty"`
	ConditionsJsonata   string   `json:"conditionsJsonata"`
	DescriptionTemplate string   `json:"descriptionTemplate"`
	HowToRemediate      string   `json:"howToRemediate,omitempty"`
	WhyShouldICare      string   `json:"whyShouldICare,omitempty"`
	Tags                []string `json:"tags,omitempty"`
}

type CreateThreatDetectionPolicyRequest struct {
	Definition       ThreatDetectionPolicyDefinition `json:"definition"`
	InstanceToStatus map[string]string               `json:"instanceToStatus,omitempty"`
}

type CreateThreatDetectionPolicyResponse struct {
	ID string `json:"id"`
}

type UpdateThreatDetectionPolicyRequest struct {
	ID         string                          `json:"id"`
	Definition ThreatDetectionPolicyDefinition `json:"definition"`
}

type ApiKey struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	CreatedBy    string   `json:"createdBy,omitempty"`
	State        string   `json:"state,omitempty"`
	PermittedIps []string `json:"permittedIps,omitempty"`
	Expiration   *string  `json:"expiration,omitempty"`
	CreatedAt    *string  `json:"createdAt,omitempty"`
}

type CreateApiKeyRequest struct {
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	PermittedIps []string `json:"permittedIps,omitempty"`
	Expiration   *string  `json:"expiration,omitempty"`
}

type CreateApiKeyResponse struct {
	Key    ApiKey `json:"key"`
	Secret string `json:"secret"`
}

type UpdateApiKeyRequest struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	PermittedIps []string `json:"permittedIps"`
}

type UpdateApiKeyResponse struct {
	Key ApiKey `json:"key"`
}

type listApiKeysResponse struct {
	Keys []ApiKey `json:"keys"`
}
