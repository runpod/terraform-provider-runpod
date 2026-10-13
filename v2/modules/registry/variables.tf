variable "name" {
  description = "Credential name."
  type        = string
  default     = "example-registry"
}

variable "username" {
  description = "Registry username. No default: pass a real login."
  type        = string
}

variable "password" {
  description = "Registry password or access token. No default: pass a real secret, never hardcode one."
  type        = string
  sensitive   = true
}

variable "ecr_resource_arn" {
  description = "AWS ECR resource ARN to delegate pulls for, e.g. arn:aws:ecr:us-east-1:123456789012:repository/trainers. Null creates no delegation."
  type        = string
  default     = null
}

variable "delegation_name" {
  description = "ECR delegation name."
  type        = string
  default     = "example-delegation"
}
