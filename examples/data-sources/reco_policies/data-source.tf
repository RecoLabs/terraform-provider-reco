data "reco_policies" "all" {}

output "policy_names" {
  value = [for p in data.reco_policies.all.policies : p.name]
}
