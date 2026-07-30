variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "volume_name" {
  type        = string
  description = "Network volume name"
  default     = "my-volume"
}

variable "volume_size_gb" {
  type        = number
  description = "Volume size in GB (min 1, max 4000)"
  default     = 10
}

variable "data_center_id" {
  type        = string
  description = "Data center ID (must support network volumes, e.g., US-MD-1, EU-RO-1)"
  default     = "US-MD-1"
}

variable "type" {
  type        = string
  description = "Storage tier: STANDARD or HIGH_PERFORMANCE"
  default     = "STANDARD"
}

variable "image_name" {
  type        = string
  description = "Docker image name"
  default     = "runpod/pytorch:1.0.7-cu1281-torch291-ubuntu2404"
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

resource "runpod_network_volume" "demo" {
  name           = var.volume_name
  size           = var.volume_size_gb
  data_center_id = var.data_center_id
  type           = var.type
}

output "volume_id" {
  description = "The network volume ID"
  value       = runpod_network_volume.demo.id
}

output "volume_info" {
  description = "Network volume details"
  value = {
    name           = runpod_network_volume.demo.name
    size_gb        = runpod_network_volume.demo.size
    data_center_id = runpod_network_volume.demo.data_center_id
    type           = runpod_network_volume.demo.type
  }
}

variable "gpu_type_id" {
  type        = string
  description = "GPU type (must exist in the volume's data center)"
  default     = "NVIDIA GeForce RTX 3090"
}

resource "runpod_pod" "with_network_volume" {
  name      = "volume-demo-pod"
  image_name      = var.image_name
  gpu_count       = var.gpu_count
  gpu_type_id     = var.gpu_type_id
  network_volume_ids = [runpod_network_volume.demo.id]
}

output "pod_id" {
  description = "Pod ID with network volume attached"
  value       = runpod_pod.with_network_volume.id
}
