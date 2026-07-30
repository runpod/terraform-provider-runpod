variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "ecr_resource_arn" {
  type        = string
  description = "ECR resource ARN with image tag (e.g., arn:aws:ecr:us-east-2:123456789:repository/myapp:latest). The API verifies access against your AWS account, so the repository must exist and the Runpod ECR delegation trust must already be configured."
}

terraform {
  required_providers {
    runpod = {
      source = "runpod/runpod"
    }
  }
}

provider "runpod" {
  api_key = var.runpod_api_key
}

resource "runpod_ecr_delegation" "demo" {
  resource = var.ecr_resource_arn
}

output "delegation_id" {
  description = "The ECR delegation ID"
  value       = runpod_ecr_delegation.demo.id
}

output "delegation_info" {
  description = "ECR delegation details"
  value = {
    resource              = runpod_ecr_delegation.demo.resource
    aws_user              = runpod_ecr_delegation.demo.aws_user
    repository            = runpod_ecr_delegation.demo.repository
    tag                   = runpod_ecr_delegation.demo.tag
    aws_region            = runpod_ecr_delegation.demo.aws_region
    docker_registry_uri   = runpod_ecr_delegation.demo.docker_registry_uri
    created_at            = runpod_ecr_delegation.demo.created_at
  }
}
