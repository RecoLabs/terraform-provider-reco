resource "reco_user" "alice" {
  email_address = "alice@example.com"
  name          = "Alice Smith"
  user_roles    = [reco_role.soc_analyst.name]
}
