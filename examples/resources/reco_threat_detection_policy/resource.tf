resource "reco_threat_detection_policy" "admin_role_granted" {
  title                = "Administrator role granted"
  description          = "Alerts when a user is granted an administrator role."
  data_source          = "GSUITE_ADMIN_AUDIT_LOG_API"
  violation_risk_level = "MEDIUM"
  violation_risk_type  = "RISK_TYPE_USER"
  default_status       = "POLICY_STATUS_PREVIEW"
  instance_to_status = {
    "11111111-1111-4111-8111-111111111111" = "POLICY_STATUS_OFF"
    "22222222-2222-4222-8222-222222222222" = "POLICY_STATUS_ON"
  }
  conditions_jsonata   = "$.eventName = 'ASSIGN_ROLE' and $contains($.roleName, 'Admin')"
  description_template = "'Administrator role granted to ' & $.targetUser"
  how_to_remediate     = "Confirm the role assignment was approved and revoke it if not."
  tags                 = ["identity", "privilege"]
}
