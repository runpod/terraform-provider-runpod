variable "name" {
  description = "Cluster display name. The only attribute that can change without replacing the cluster."
  type        = string
  default     = "example-cluster"
}

variable "type" {
  description = "Cluster type: TRAINING, SLURM, RAY, or APPLICATION."
  type        = string
  default     = "TRAINING"

  validation {
    condition     = var.type == null ? true : contains(["TRAINING", "SLURM", "RAY", "APPLICATION"], var.type)
    error_message = "type must be TRAINING, SLURM, RAY, or APPLICATION."
  }
}

variable "gpu_type_id" {
  description = "GPU type every pod in the cluster runs on (see the catalog-discovery module)."
  type        = string
  default     = "NVIDIA H100 80GB HBM3"
}

variable "gpu_count_per_pod" {
  description = "GPUs on each pod. Total GPUs are pod_count * gpu_count_per_pod."
  type        = number
  default     = 1

  validation {
    condition     = var.gpu_count_per_pod == null ? true : var.gpu_count_per_pod >= 1
    error_message = "gpu_count_per_pod must be at least 1."
  }
}

variable "pod_count" {
  description = "Pods (nodes) in the cluster."
  type        = number
  default     = 2

  validation {
    condition     = var.pod_count == null ? true : var.pod_count >= 1
    error_message = "pod_count must be at least 1."
  }
}

variable "data_center_ids" {
  description = "Preferred data centers for placement. Null lets the scheduler choose."
  type        = list(string)
  default     = null
}

variable "image" {
  description = "Docker image every member pod runs. Public images only; a cluster takes no registry credential."
  type        = string
  default     = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
}

variable "args" {
  description = "Arguments passed to the container entrypoint on every member pod. Null keeps the image's own."
  type        = string
  default     = null
}

variable "disk" {
  description = "Container disk in GB on each pod (ephemeral)."
  type        = number
  default     = 50

  validation {
    condition     = var.disk == null ? true : var.disk > 0
    error_message = "disk must be greater than 0 GB."
  }
}

variable "env" {
  description = "Environment variables set on every member pod."
  type        = map(string)
  default     = {}
}

variable "ports" {
  description = "Exposed ports on every member pod."
  type        = list(string)
  default     = ["22/tcp"]
}

variable "start_ssh" {
  description = "Provision SSH on every member pod using the account's registered keys (see the account-ssh-keys module)."
  type        = bool
  default     = true
}

variable "start_jupyter" {
  description = "Start JupyterLab on every member pod. Null uses the API default."
  type        = bool
  default     = null
}
