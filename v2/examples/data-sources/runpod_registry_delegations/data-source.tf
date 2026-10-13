data "runpod_registry_delegations" "all" {}

output "delegated_repositories" {
  description = "Registry URI each delegated ECR repository resolves through."
  value = {
    for d in data.runpod_registry_delegations.all.delegations :
    "${d.repository}:${d.tag}" => d.docker_registry_uri
  }
}

output "aws_principals_to_authorize" {
  description = "AWS principals Runpod pulls as, per region; grant each read access on its repository."
  value       = toset([for d in data.runpod_registry_delegations.all.delegations : "${d.aws_user} (${d.aws_region})"])
}
