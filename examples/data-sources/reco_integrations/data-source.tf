data "reco_integrations" "all" {}

output "connected_integration_names" {
  value = [for i in data.reco_integrations.all.integrations : i.app]
}
