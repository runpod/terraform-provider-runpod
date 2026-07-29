variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "machine_id" {
  type        = string
  description = "Machine ID to deploy pod on (v1 only, ignored in v2)"
  default     = ""
}

variable "image_name" {
  type        = string
  description = "Docker image name (use template_id or image_name, not both)"
  default     = ""
}

variable "template_id" {
  type        = string
  description = "Template ID to use for pod creation"
  default     = "6cqbth7fkj"
}

variable "pod_name" {
  type        = string
  description = "Pod name"
  default     = "demo-pod"
}

variable "gpu_type_id" {
  type        = string
  description = "GPU type ID (required in v2)"
  default     = "NVIDIA GeForce RTX 3090"
}

variable "gpu_count" {
  type        = number
  description = "Number of GPUs"
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

resource "runpod_pod" "demo" {
  template_id   = var.template_id
  gpu_type_id   = "NVIDIA GeForce RTX 3090"
  gpu_count     = var.gpu_count
  name          = var.pod_name
}

output "pod_id" {
  description = "The pod ID"
  value       = runpod_pod.demo.id
}

output "pod_status" {
  description = "Current pod status"
  value       = runpod_pod.demo.status
}

output "pod_machine_id" {
  description = "Machine ID where pod is running"
  value       = runpod_pod.demo.machine_id
}

output "pod_gpu_type" {
  description = "GPU type allocated"
  value       = runpod_pod.demo.gpu_type_id
}
