# Deprecated GraphQL shim; removed when REST v2 has an account route.

data "runpod_user" "this" {}

# Only id and email have a GraphQL source; the 1.x quota, key, name, verified and
# cloud-tier attributes read null.
output "account_email" {
  description = "Email of the account the API key belongs to."
  value       = data.runpod_user.this.email
}
