data "reco_users" "all" {}

output "admin_emails" {
  value = [for u in data.reco_users.all.users : u.email_address if contains(u.user_roles, "Admin")]
}
