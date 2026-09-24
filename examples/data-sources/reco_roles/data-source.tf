data "reco_roles" "all" {}

output "custom_role_names" {
  value = [for r in data.reco_roles.all.roles : r.name if r.type == "USER_ROLE_TYPE_CUSTOM"]
}
