# RunPod PyTorch Pod Example
# 
# You can deploy using either:
# 1. A template_id (uses a pre-configured template)
# 2. An image_name + gpu_type_id (deploys directly from Docker image)
#
# If template_id is provided, it will be used. Otherwise, image_name and gpu_type_id are required.

variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "template_id" {
  type        = string
  description = "Template ID for the pod (get from https://www.runpod.io/console/templates)"
  default     = "6cqbth7fkj"
}

variable "image_name" {
  type        = string
  description = "Docker image name for direct deployment (alternative to template_id)"
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

resource "runpod_pod" "pytorch_experiment" {
  template_id   = var.template_id
  gpu_type_id   = "NVIDIA GeForce RTX 3090"
  gpu_count     = 1
  name          = "pytorch-experiment"
}

# Output pod details
output "pod_id" {
  value = runpod_pod.pytorch_experiment.id
  description = "The pod ID for your PyTorch experiment"
}

output "pod_status" {
  value = runpod_pod.pytorch_experiment.status
  description = "Current pod status"
}
