variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "endpoint_id" {
  type        = string
  description = "Endpoint ID to run the job on"
}

variable "input" {
  type        = string
  description = "JSON input for the endpoint (e.g., {\"prompt\": \"hello world\"})"
  default     = ""
}

variable "template_id" {
  type        = string
  description = "Template ID (optional, for serverless endpoints)"
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

resource "runpod_endpoint_job" "demo" {
  endpoint_id = var.endpoint_id
  input       = var.input
  template_id = var.template_id
}

output "job_id" {
  description = "The endpoint job ID"
  value       = runpod_endpoint_job.demo.id
}

output "job_status" {
  description = "Job status"
  value       = runpod_endpoint_job.demo.status
}

output "job_output" {
  description = "Job output"
  value       = runpod_endpoint_job.demo.output
}

output "job_info" {
  description = "Job details"
  value = {
    id              = runpod_endpoint_job.demo.id
    endpoint_id     = runpod_endpoint_job.demo.endpoint_id
    status          = runpod_endpoint_job.demo.status
    created_at      = runpod_endpoint_job.demo.created_at
    completed_at    = runpod_endpoint_job.demo.completed_at
    duration_ms     = runpod_endpoint_job.demo.duration_ms
    output          = runpod_endpoint_job.demo.output
  }
}
