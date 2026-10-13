output "id" {
  description = "Credential identifier, used as the registry input of the pod, endpoint and template modules."
  value       = runpod_registry.this.id
}

output "registry_ids" {
  description = "Identifiers of every registry credential on the account, including this one."
  value       = [for r in data.runpod_registries.all.registries : r.id]
}

output "registry_usernames" {
  description = "Deprecated. Credential id to null for every credential on the account, kept for 1.x configurations: no supported API reports a registry username."
  value       = { for r in data.runpod_registries.all.registries : r.id => r.username }
}

output "delegation_id" {
  description = "Identifier of the ECR delegation; null when no ARN was given."
  value       = try(one(runpod_registry_delegation.this).id, null)
}

output "docker_registry_uri" {
  description = "Registry URI images resolve through once delegated; null when no ARN was given."
  value       = try(one(runpod_registry_delegation.this).docker_registry_uri, null)
}

output "delegations" {
  description = "Every ECR delegation on the account, with the AWS region and principal to authorize."
  value       = data.runpod_registry_delegations.all.delegations
}

output "delegation_ids" {
  description = "Identifiers of every ECR delegation on the account."
  value       = [for d in data.runpod_registry_delegations.all.delegations : d.id]
}
