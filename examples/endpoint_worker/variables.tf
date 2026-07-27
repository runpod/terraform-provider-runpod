variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "endpoint_id" {
  type        = string
  description = "Endpoint ID to list workers from"
}
