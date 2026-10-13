variable "gpu_type_id" {
  description = "GPU type to look up in detail, e.g. for its Serverless pool and prices."
  type        = string
  default     = "NVIDIA GeForce RTX 4090"
}

variable "cpu_type_id" {
  description = "CPU flavor to look up in detail. Null skips the lookup, since flavor identifiers vary by account visibility."
  type        = string
  default     = null
}

variable "data_center_id" {
  description = "Data center to look up in detail."
  type        = string
  default     = "EU-RO-1"
}

variable "catalog_slice" {
  description = "Public template catalog slice to read: official (Runpod-curated), verified (Runpod-verified community templates), or community (everything else shared publicly)."
  type        = string
  default     = "official"

  validation {
    condition     = var.catalog_slice == null ? true : contains(["official", "verified", "community"], var.catalog_slice)
    error_message = "catalog_slice must be official, verified, or community."
  }
}
