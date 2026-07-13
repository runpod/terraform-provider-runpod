variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
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

# List available GPU types
data "runpod_gpu_types" "all" {}

output "gpu_types" {
  description = "Available GPU types"
  value       = data.runpod_gpu_types.all
}

# List available data centers
data "runpod_data_centers" "locations" {}

output "data_centers" {
  description = "Available data centers"
  value       = data.runpod_data_centers.locations
}

# Get current user info
data "runpod_user" "current" {}

output "user_id" {
  description = "Current user ID"
  value       = data.runpod_user.current.id
}

output "public_key" {
  description = "User's public SSH key"
  value       = data.runpod_user.current.pub_key
}

# List available machines (returns all machines - listed and unlisted)
data "runpod_machines" "available" {}

output "available_machines" {
  description = "Available machines (first 5 shown)"
  value       = data.runpod_machines.available.machines[:5]
}
