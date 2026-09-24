data "reco_posture_checks" "all" {}

output "posture_check_names" {
  value = [for c in data.reco_posture_checks.all.posture_checks : c.name]
}
