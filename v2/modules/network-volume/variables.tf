variable "name" {
  description = "Volume name."
  type        = string
  default     = "example-volume"
}

variable "size" {
  description = "Storage to allocate in GB, 10 to 4096. Can grow, never shrink."
  type        = number
  default     = 10

  validation {
    condition     = var.size == null ? true : (var.size >= 10 && var.size <= 4096)
    error_message = "size must be between 10 and 4096 GB."
  }
}

variable "data_center" {
  description = "Data center to create the volume in (see the catalog-discovery module). Immutable; pods and workers that mount the volume have to be placed there."
  type        = string
  default     = "EU-RO-1"
}

variable "type" {
  description = "Storage tier: STANDARD or HIGH_PERFORMANCE. Null uses the data center default."
  type        = string
  default     = null

  validation {
    condition     = var.type == null ? true : contains(["STANDARD", "HIGH_PERFORMANCE"], var.type)
    error_message = "type must be STANDARD or HIGH_PERFORMANCE."
  }
}
