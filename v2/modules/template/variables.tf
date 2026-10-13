variable "name" {
  description = "Template name."
  type        = string
  default     = "example-template"
}

variable "image" {
  description = "Docker image reference."
  type        = string
  default     = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
}

variable "args" {
  description = "Arguments passed to the container entrypoint. Null keeps the image's own."
  type        = string
  default     = null
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

variable "env" {
  description = "Environment variables."
  type        = map(string)
  default     = { EXAMPLE = "true" }
}

variable "ports" {
  description = "Exposed ports."
  type        = list(string)
  default     = ["8888/http", "22/tcp"]
}

variable "serverless" {
  description = "Whether the template configures Serverless workers (true) or pods (false). Null uses the API default, which is pods."
  type        = bool
  default     = null
}

variable "public" {
  description = "Whether the template is shared publicly in the catalog. Null uses the API default, which is private."
  type        = bool
  default     = null
}

variable "category" {
  description = "Console grouping of a public template: CPU, NVIDIA, or AMD. Null leaves it unset."
  type        = string
  default     = null

  validation {
    condition     = var.category == null ? true : contains(["CPU", "NVIDIA", "AMD"], var.category)
    error_message = "category must be CPU, NVIDIA, or AMD."
  }
}

variable "registry" {
  description = "Registry credential ID (see the registry module) for pulling private images, called container_registry_auth_id in 1.x. Null for public images."
  type        = string
  default     = null
}

variable "start_ssh" {
  description = "Provision SSH access in containers created from this template. Null uses the API default, which is true."
  type        = bool
  default     = null
}

variable "start_jupyter" {
  description = "Start JupyterLab in containers created from this template. Null uses the API default, which is true."
  type        = bool
  default     = null
}

variable "allowed_cuda_versions" {
  description = "Exact set of acceptable host CUDA versions for containers created from this template, e.g. [\"12.8\"]. Null accepts any version."
  type        = list(string)
  default     = null
}

variable "persistent_volume" {
  description = "Host-local persistent storage for containers created from this template, replacing the 1.x volume_in_gb and volume_mount_path attributes. Null attaches none."
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

variable "docker_entrypoint" {
  description = "Deprecated. Container entrypoint for containers created from this template, e.g. [\"/bin/bash\"], preserving the 1.x docker_entrypoint attribute. Null leaves the template's own value in place."
  type        = list(string)
  default     = null
}

variable "readme" {
  description = "Deprecated. Markdown description shown on the template's console page, preserving the 1.x readme attribute. Null leaves the template's own text in place; \"\" clears it."
  type        = string
  default     = null
}
