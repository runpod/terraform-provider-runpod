variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
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

# Note: Machines data source requires GraphQL which is not exposed in v2 REST API
# Use the RunPod console to view available machines
