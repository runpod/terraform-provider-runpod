variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "auth_name" {
  type        = string
  description = "Authentication name (must be unique)"
}

variable "registry_username" {
  type        = string
  description = "Registry username"
}

variable "registry_password" {
  type        = string
  description = "Registry password or access token"
  sensitive   = true
}
