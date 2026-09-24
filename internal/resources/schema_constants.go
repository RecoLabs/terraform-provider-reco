package resources

const (
	attrPermission   = "permission"
	attrResource     = "resource"
	attrResourceName = "resource_name"
	attrName         = "name"
	attrDescription  = "description"
	attrCreatedAt    = "created_at"

	writeOnlyNote = "The Reco API does not return this value, so changes made outside Terraform are not detected and it is empty after import."
)

var (
	riskLevels = []string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}
	riskTypes  = []string{
		"RISK_TYPE_UNKNOWN",
		"RISK_TYPE_DATA",
		"RISK_TYPE_USER",
		"RISK_TYPE_USER_TARGET",
		"RISK_TYPE_USER_GROUP",
		"RISK_TYPE_APPLICATION",
	}
	policyStatuses = []string{"POLICY_STATUS_OFF", "POLICY_STATUS_PREVIEW", "POLICY_STATUS_ON"}
)
