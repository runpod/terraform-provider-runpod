variable "name" {
  description = "Pod display name."
  type        = string
  default     = "example-cpu-pod"
}

variable "image" {
  description = "Docker image reference. Null when template_id supplies one."
  type        = string
  default     = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
}

variable "template_id" {
  description = "Pod template to base the pod on (see the template module). Null builds the pod from image alone."
  type        = string
  default     = null
}

variable "args" {
  description = "Arguments passed to the container entrypoint. Null keeps the image's own."
  type        = string
  default     = null
}

variable "cpu_type_id" {
  description = "CPU flavor from the catalog (see the catalog-discovery module)."
  type        = string
  default     = "cpu5c"
}

variable "vcpu_count" {
  description = "Number of vCPUs. Must be valid for the flavor and a power of two."
  type        = number
  default     = 4
}

variable "cloud" {
  description = "Cloud tier: SECURE or COMMUNITY."
  type        = string
  default     = "SECURE"

  validation {
    condition     = var.cloud == null ? true : contains(["SECURE", "COMMUNITY"], var.cloud)
    error_message = "cloud must be SECURE or COMMUNITY."
  }
}

variable "disk" {
  description = "Container disk in GB (ephemeral)."
  type        = number
  default     = 20

  validation {
    condition     = var.disk == null ? true : var.disk > 0
    error_message = "disk must be greater than 0 GB."
  }
}

variable "data_center_ids" {
  description = "Preferred data centers for placement. Null lets the scheduler choose; a network volume requires the pod to land in the volume's data center."
  type        = list(string)
  default     = null
}

variable "registry" {
  description = "Registry credential ID (see the registry module) for pulling a private image. Null for public images."
  type        = string
  default     = null
}

variable "desired_state" {
  description = "Managed run state: RUNNING or EXITED. When set, every apply starts or stops the pod to match, and a pod stopped outside Terraform shows up as a plan diff. Null leaves the run state unmanaged."
  type        = string
  default     = null

  validation {
    condition     = var.desired_state == null ? true : contains(["RUNNING", "EXITED"], var.desired_state)
    error_message = "desired_state must be RUNNING or EXITED."
  }
}

variable "network_volumes" {
  description = "Network volumes to mount (see the network-volume module), replacing the 1.x network_volume_id attribute. Each volume must live in the same data center as the pod. Null attaches none, and the API takes one entry."
  type = list(object({
    volume_id = string
    path      = string
  }))
  default = null
}

variable "start_ssh" {
  description = "Set up SSH access at create time. Needs an SSH key registered on the account (see the account-ssh-keys module), and \"22/tcp\" in ports for the direct form."
  type        = bool
  default     = true
}

variable "start_jupyter" {
  description = "Start JupyterLab at create time, reachable on the \"8888/http\" port."
  type        = bool
  default     = true
}

variable "env" {
  description = "Environment variables."
  type        = map(string)
  default     = {}
}

variable "ports" {
  description = "Exposed ports."
  type        = list(string)
  default     = ["8888/http", "22/tcp"]
}
