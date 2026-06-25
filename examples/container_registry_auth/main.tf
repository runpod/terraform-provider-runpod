variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "auth_name" {
  type        = string
  description = "Authentication name (must be unique)"
}

variable "registry_username" {
  type        = string
  description = "Registry username"
}

variable "registry_password" {
  type        = string
  description = "Registry password or access token"
  sensitive   = true
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

resource "runpod_container_registry_auth" "demo" {
  name       = var.auth_name
  username   = var.registry_username
  password   = var.registry_password
}

output "auth_id" {
  description = "The container registry auth ID"
  value       = runpod_container_registry_auth.demo.id
}

output "auth_info" {
  description = "Authentication details"
  value = {
    name = runpod_container_registry_auth.demo.name
  }
}
