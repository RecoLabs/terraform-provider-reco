resource "reco_posture_check" "mfa_enforced" {
  title                  = "MFA is enforced for all users"
  data_source            = "GSUITE_USERS_API"
  violation_risk_level   = "HIGH"
  violation_risk_type    = "RISK_TYPE_USER"
  conditions_jsonata     = "payload.isEnrolledIn2Sv = false"
  posture_unique_jsonata = "$.primaryEmail"
  posture_value_jsonata  = "$.primaryEmail"
}

# The controls are the framework's complete list: every apply replaces the
# controls in Reco with these, so keep all of them here.
resource "reco_compliance_framework" "internal_controls" {
  title       = "Internal Security Controls"
  description = "Our baseline security controls."
  version     = "2026.1"
  type        = "internal"

  controls = [
    {
      control_id  = "AC-1"
      title       = "Access control"
      description = "Access to systems is restricted to authorised users."
    },
    {
      control_id         = "AC-1.1"
      title              = "Multi-factor authentication"
      parent_control_id  = "AC-1"
      related_policy_ids = [reco_posture_check.mfa_enforced.id]
    },
    {
      control_id = "AC-2"
      title      = "Quarterly access review"
      manual     = true
    },
  ]
}
