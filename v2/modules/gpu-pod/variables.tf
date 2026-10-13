variable "name" {
  description = "Pod display name."
  type        = string
  default     = "example-gpu-pod"
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

variable "gpu_type_id" {
  description = "GPU type from the catalog (see the catalog-discovery module)."
  type        = string
  default     = "NVIDIA GeForce RTX 4090"
}

variable "gpu_count" {
  description = "Number of GPUs."
  type        = number
  default     = 1
}

variable "allowed_cuda_versions" {
  description = "Exact set of acceptable host CUDA versions, e.g. [\"12.8\"]. Null accepts any; a non-empty set cannot be combined with min_cuda_version."
  type        = list(string)
  default     = null
}

variable "min_cuda_version" {
  description = "Lowest acceptable host CUDA version, e.g. \"12.1\". Null sets no floor."
  type        = string
  default     = null
}

variable "min_ram_per_gpu" {
  description = "Minimum host system RAM in GB per requested GPU. A placement filter: raising it reduces the machines that can take the pod."
  type        = number
  default     = null
}

variable "min_vcpu_count_per_gpu" {
  description = "Minimum host vCPU count per requested GPU. A placement filter: raising it reduces the machines that can take the pod."
  type        = number
  default     = null
}

variable "cloud" {
  description = "Cloud tier: SECURE or COMMUNITY. COMMUNITY is cheaper but capacity for a given GPU type fluctuates."
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

variable "global_networking" {
  description = "Enable global networking (a private cross-datacenter IP). Needs an eligible data center. Null leaves it off."
  type        = bool
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

variable "persistent_volume" {
  description = "Host-local persistent storage, replacing the 1.x volume_in_gb and volume_mount_path attributes. Null attaches none. Mutually exclusive with network_volumes."
  type = object({
    size = number
    path = string
  })
  default = null

  validation {
    condition     = var.persistent_volume == null ? true : var.persistent_volume.size >= 10
    error_message = "persistent_volume.size must be 10 GB or more."
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
