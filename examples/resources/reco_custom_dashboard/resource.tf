resource "reco_custom_dashboard" "security_overview" {
  name        = "Security Overview"
  visibility  = "DASHBOARD_VISIBILITY_SHARED_READ"
  folder_path = "/team/security"

  # A dashboard has sections, each with cells, each holding widgets.
  # Widget keys are listed by GET /api/v1/external-api/dashboards/widgets.
  spec = jsonencode({
    sections = [{
      columns = 2
      cells = [{
        colSpan = 1
        widgets = [{ widget = "DASHBOARD_WIDGET_TOTAL_SAAS_APPS" }]
      }]
    }]
  })
}
