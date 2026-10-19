resource "reco_custom_dashboard" "security_overview" {
  name        = "Security Overview"
  visibility  = "DASHBOARD_VISIBILITY_SHARED_READ"
  folder_path = "/team/security"

  # Widget keys are listed by GET /api/v1/external-api/dashboards/widgets.
  spec = jsonencode({
    widgets = ["DASHBOARD_WIDGET_TOTAL_SAAS_APPS"]
  })
}
