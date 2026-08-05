variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "template_name" {
  type        = string
  description = "Template name (must be unique)"
  default     = "my-template"
}

variable "image_name" {
  type        = string
  description = "Docker image name"
  default     = "runpod/echo-server"
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
  default     = 0
}

variable "volume_mount_path" {
  type        = string
  description = "Volume mount path"
  default     = ""
}
