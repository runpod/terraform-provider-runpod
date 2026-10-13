resource "runpod_registry_delegation" "ecr" {
  name     = "ecr-trainers"
  resource = "arn:aws:ecr:us-east-1:123456789012:repository/trainers"
}

output "docker_registry_uri" {
  description = "Registry URI images resolve through once delegated."
  value       = runpod_registry_delegation.ecr.docker_registry_uri
}
