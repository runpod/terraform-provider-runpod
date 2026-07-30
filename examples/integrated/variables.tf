variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "template_name" {
  type        = string
  description = "Name for the template"
  default     = "my-pytorch-template"
}

variable "image_name" {
  type        = string
  description = "Docker image for the template"
  default     = "runpod/pytorch:2.0.1-py3.11-cuda12.1-ubuntu22.04"
}

variable "volume_name" {
  type        = string
  description = "Network volume name"
  default     = "my-data-volume"
}

variable "volume_size_gb" {
  type        = number
  description = "Volume size in GB"
  default     = 50
}

variable "data_center_id" {
  type        = string
  description = "Data center ID"
  default     = "US-MD-1"
}

variable "gpu_count" {
  type        = number
  description = "Number of GPUs"
  default     = 1
}
