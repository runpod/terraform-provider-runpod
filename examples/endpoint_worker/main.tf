variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "endpoint_id" {
  type        = string
  description = "Endpoint ID to list workers from"
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

resource "runpod_endpoint_worker" "demo" {
  endpoint_id = var.endpoint_id
}

output "worker_id" {
  description = "The endpoint worker ID"
  value       = runpod_endpoint_worker.demo.id
}

output "worker_status" {
  description = "Worker status"
  value       = runpod_endpoint_worker.demo.status
}

output "worker_info" {
  description = "Worker details"
  value = {
    id               = runpod_endpoint_worker.demo.id
    endpoint_id      = runpod_endpoint_worker.demo.endpoint_id
    pod_id           = runpod_endpoint_worker.demo.pod_id
    container_id     = runpod_endpoint_worker.demo.container_id
    status           = runpod_endpoint_worker.demo.status
    start_time       = runpod_endpoint_worker.demo.start_time
    idle_seconds     = runpod_endpoint_worker.demo.idle_seconds
    uptime_ms        = runpod_endpoint_worker.demo.uptime_ms
    last_busy_ms     = runpod_endpoint_worker.demo.last_busy_ms
  }
}
