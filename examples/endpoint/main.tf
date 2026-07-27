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
  image_name   = var.image_name
  gpu_type_id  = var.gpu_type_id
  name         = var.endpoint_name
  workers_min  = var.workers_min
  workers_max  = var.workers_max
  idle_timeout = var.idle_timeout
  gpu_count    = var.gpu_count
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
    name                 = runpod_endpoint.demo.name
    image_name           = runpod_endpoint.demo.image_name
    gpu_type_id          = runpod_endpoint.demo.gpu_type_id
    workers_min          = runpod_endpoint.demo.workers_min
    workers_max          = runpod_endpoint.demo.workers_max
    idle_timeout         = runpod_endpoint.demo.idle_timeout
    gpu_count            = runpod_endpoint.demo.gpu_count
    cloud_type           = runpod_endpoint.demo.cloud_type
    container_disk_in_gb = runpod_endpoint.demo.container_disk_in_gb
  }
}
