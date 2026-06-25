variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "template_id" {
  type        = string
  description = "Template ID to use for the endpoint"
}

variable "endpoint_name" {
  type        = string
  description = "Endpoint name"
  default     = "my-endpoint"
}

variable "workers_min" {
  type        = number
  description = "Minimum number of workers"
  default     = 0
}

variable "workers_max" {
  type        = number
  description = "Maximum number of workers"
  default     = 3
}

variable "idle_timeout" {
  type        = number
  description = "Idle timeout in seconds"
  default     = 5
}

variable "gpu_count" {
  type        = number
  description = "Number of GPUs per worker"
  default     = 1
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

resource "runpod_endpoint" "demo" {
  template_id    = var.template_id
  name           = var.endpoint_name
  workers_min    = var.workers_min
  workers_max    = var.workers_max
  idle_timeout   = var.idle_timeout
  gpu_count      = var.gpu_count
}

output "endpoint_id" {
  description = "The endpoint ID"
  value       = runpod_endpoint.demo.id
}

output "endpoint_url" {
  description = "The endpoint URL for API calls"
  value       = "https://api.runpod.ai/v2/${runpod_endpoint.demo.id}/runsync"
}

output "endpoint_info" {
  description = "Endpoint configuration details"
  value = {
    name           = runpod_endpoint.demo.name
    template_id    = runpod_endpoint.demo.template_id
    workers_min    = runpod_endpoint.demo.workers_min
    workers_max    = runpod_endpoint.demo.workers_max
    idle_timeout   = runpod_endpoint.demo.idle_timeout
    gpu_count      = runpod_endpoint.demo.gpu_count
    compute_type   = runpod_endpoint.demo.compute_type
  }
}
