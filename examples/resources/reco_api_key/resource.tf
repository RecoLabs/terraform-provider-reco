resource "reco_api_key" "ci" {
  name          = "ci-pipeline"
  role          = reco_role.soc_analyst.name
  permitted_ips = ["203.0.113.10/32"]
  expiration    = "2027-12-31T00:00:00Z"
}

resource "reco_api_key" "retired" {
  name  = "ci-pipeline-retired"
  role  = reco_role.soc_analyst.name
  state = "APPLICATION_KEY_STATE_RETIRED"
}

output "ci_api_key_secret" {
  value     = reco_api_key.ci.secret
  sensitive = true
}
