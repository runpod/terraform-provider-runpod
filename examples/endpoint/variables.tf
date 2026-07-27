variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "image_name" {
  type        = string
  description = "Container image name (v2 required)"
  default     = "runpod/echo-server"
}

variable "gpu_type_id" {
  type        = string
  description = "GPU type ID (v2 required)"
  default     = "NVIDIA A100-SXM-80GB"
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
  description = "Idle timeout in minutes"
  default     = 5
}

variable "gpu_count" {
  type        = number
  description = "Number of GPUs per worker"
  default     = 1
}
