variable "runpod_api_key" {
  type        = string
  description = "RunPod API key"
  sensitive   = true
  default     = ""
}

variable "pod_id" {
  type        = string
  description = "Pod ID to monitor"
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

data "runpod_pod" "monitor" {
  id = var.pod_id
}

output "pod_info" {
  description = "Pod information"
  value = {
    id              = data.runpod_pod.monitor.id
    name            = data.runpod_pod.monitor.name
    status          = data.runpod_pod.monitor.status
    gpu_count       = data.runpod_pod.monitor.gpu_count
    gpu_type_id     = data.runpod_pod.monitor.gpu_type_id
    machine_id      = data.runpod_pod.monitor.machine_id
    image_name      = data.runpod_pod.monitor.image_name
    cost_per_hr     = data.runpod_pod.monitor.cost_per_hr
    created_at      = data.runpod_pod.monitor.created_at
    desired_status  = data.runpod_pod.monitor.desired_status
  }
}
