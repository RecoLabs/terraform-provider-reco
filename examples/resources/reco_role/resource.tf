resource "reco_role" "soc_analyst" {
  name        = "SOC Analyst"
  description = "Read-only access for SOC team members."
  permissions = ["PERM_ALERTS_READ", "PERM_EVENT_READ", "PERM_POLICIES_READ"]
}
