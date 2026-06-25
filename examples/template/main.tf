variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "template_name" {
  type        = string
  description = "Template name (must be unique)"
}

variable "image_name" {
  type        = string
  description = "Docker image name"
}

variable "category" {
  type        = string
  description = "Compute category: NVIDIA, AMD, or CPU"
  default     = "NVIDIA"
}

variable "is_public" {
  type        = bool
  description = "Whether template is public"
  default     = false
}

variable "is_serverless" {
  type        = bool
  description = "Whether template is for serverless endpoints"
  default     = false
}

variable "container_disk_in_gb" {
  type        = number
  description = "Container disk size in GB"
  default     = 50
}

variable "volume_in_gb" {
  type        = number
  description = "Volume size in GB"
  default     = 20
}

variable "volume_mount_path" {
  type        = string
  description = "Volume mount path"
  default     = "/workspace"
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

resource "runpod_template" "demo" {
  name                 = var.template_name
  image_name           = var.image_name
  category             = var.category
  is_public            = var.is_public
  is_serverless        = var.is_serverless
  container_disk_in_gb = var.container_disk_in_gb
  volume_in_gb         = var.volume_in_gb
  volume_mount_path    = var.volume_mount_path
}

output "template_id" {
  description = "The template ID"
  value       = runpod_template.demo.id
}

output "template_info" {
  description = "Template details"
  value = {
    name         = runpod_template.demo.name
    image_name   = runpod_template.demo.image_name
    category     = runpod_template.demo.category
    is_public    = runpod_template.demo.is_public
    is_serverless = runpod_template.demo.is_serverless
  }
}
