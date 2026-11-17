resource "reco_threat_detection_policy" "admin_role_granted" {
  title                = "Administrator role granted"
  description          = "Alerts when a user is granted an administrator role."
  data_source          = "GSUITE_ADMIN_AUDIT_LOG_API"
  violation_risk_level = "MEDIUM"
  violation_risk_type  = "RISK_TYPE_USER"
  default_status       = "POLICY_STATUS_PREVIEW"
  conditions_jsonata   = "$.eventName = 'ASSIGN_ROLE' and $contains($.roleName, 'Admin')"
  description_template = "'Administrator role granted to ' & $.targetUser"
}

resource "reco_threat_detection_exclusion" "service_account" {
  name        = "Trusted service account"
  description = "Suppress alerts for the break-glass automation account."
  expression  = "$.actor.email = 'svc-breakglass@example.com'"
  policy_ids  = [reco_threat_detection_policy.admin_role_granted.id]
}

resource "reco_threat_detection_exclusion" "maintenance_window" {
  name        = "Maintenance window"
  expression  = "$.eventName = 'ASSIGN_ROLE'"
  is_global   = true
  state       = "VIOLATION_EXCLUSION_STATE_ON_TEMPORARY"
  valid_until = "2026-12-31T00:00:00Z"
}
