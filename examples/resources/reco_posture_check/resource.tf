resource "reco_posture_check" "admins_without_mfa" {
  title                  = "Admin accounts without MFA"
  description            = "Flags administrator accounts that have not enrolled in multi-factor authentication."
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "HIGH"
  violation_risk_type    = "RISK_TYPE_USER"
  default_status         = "POLICY_STATUS_PREVIEW"
  posture_unique_jsonata = "$.primaryEmail"
  posture_value_jsonata  = "$.primaryEmail"
  conditions_jsonata     = "$.isAdmin = true and $.isEnrolledIn2Sv = false"
  how_to_remediate       = "Require the administrator to enroll in 2-Step Verification."
  why_should_i_care      = "Administrator accounts without MFA are a common entry point for account takeover."
  tags                   = ["identity", "mfa"]
}
