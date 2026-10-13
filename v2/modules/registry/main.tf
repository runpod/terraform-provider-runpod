resource "runpod_registry" "this" {
  name     = var.name
  username = var.username
  password = var.password
}

# An AWS ECR pull delegation, when an ARN is given. The API never echoes the
# ARN back, so the resource is the only place `resource` is visible.
resource "runpod_registry_delegation" "this" {
  count = var.ecr_resource_arn == null ? 0 : 1

  name     = var.delegation_name
  resource = var.ecr_resource_arn
}

# Every credential and every delegation on the account, including the ones
# above. Secrets are never returned.
data "runpod_registries" "all" {
  depends_on = [runpod_registry.this]
}

data "runpod_registry_delegations" "all" {
  depends_on = [runpod_registry_delegation.this]
}
