resource "reco_custom_compliance_framework" "internal_controls" {
  title       = "Internal Security Controls"
  description = "Framework for compliance as defined in corporate guidelines spec 7984."
  version     = "1.0"
  is_active   = true

  controls = [
    {
      control_id = "ACC"
      title      = "Access control"
    },
    {
      control_id         = "ACC-1"
      title              = "MFA is enforced for all users"
      parent_control_id  = "ACC"
      related_policy_ids = ["11111111-1111-1111-1111-111111111111"] # posture check IDs
    },
    {
      control_id        = "ACC-2"
      title             = "Quarterly access review"
      description       = "Reviewed manually by the security team."
      parent_control_id = "ACC"
      manual            = true
    },
  ]
}
