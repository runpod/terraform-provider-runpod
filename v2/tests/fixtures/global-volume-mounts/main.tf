# One global volume mounted on a GPU pod and on a GPU endpoint scaled to zero
# workers, for global_volume.tftest.hcl.

terraform {
  required_providers {
    runpod = {
      source = "runpod/runpod"
    }
  }
}

variable "volume_id" {
  type = string
}

variable "gpu_type_id" {
  type = string
}

variable "pod_path" {
  type = string
}

variable "endpoint_volume_ids" {
  type = list(string)
}

resource "runpod_pod" "this" {
  name        = "tf-test-pod-global-volume"
  image       = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  gpu_type_id = var.gpu_type_id
  cloud       = "SECURE"
  disk        = 20

  global_volumes = [{
    volume_id = var.volume_id
    path      = var.pod_path
  }]
}

resource "runpod_serverless_endpoint" "this" {
  name          = "tf-test-endpoint-global-volume"
  image         = "runpod/mock-worker:dev"
  endpoint_type = "QUEUE"
  gpu_pools     = ["ADA_24"]
  scaling_type  = "QUEUE_DELAY"
  scaling_value = 4
  workers_min   = 0
  workers_max   = 1

  global_volumes = var.endpoint_volume_ids
}

output "pod_id" {
  value = runpod_pod.this.id
}

output "pod_global_volumes" {
  value = runpod_pod.this.global_volumes
}

output "endpoint_id" {
  value = runpod_serverless_endpoint.this.id
}

output "endpoint_global_volumes" {
  value = runpod_serverless_endpoint.this.global_volumes
}
