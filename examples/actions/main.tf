variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
}

variable "pod_id" {
  type        = string
  description = "Pod ID to manage"
}

variable "action" {
  type        = string
  description = "Action to perform: start, stop, restart, terminate"
  default     = "stop"
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

resource "runpod_pod_action" "manage" {
  pod_id = var.pod_id
  action = var.action
}

output "action_status" {
  description = "Status after action"
  value       = runpod_pod_action.manage.status
}
