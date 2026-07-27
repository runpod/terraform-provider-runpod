variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "ecr_resource_arn" {
  type        = string
  description = "ECR resource ARN (e.g., arn:aws:ecr:us-east-2:123456789:repository/myapp)"
}
