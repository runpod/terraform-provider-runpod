variable "bucket_size" {
  description = "Length of each time bucket: hour, day, week, month, or year."
  type        = string
  default     = "day"

  validation {
    condition     = var.bucket_size == null ? true : contains(["hour", "day", "week", "month", "year"], var.bucket_size)
    error_message = "bucket_size must be hour, day, week, month, or year."
  }
}

variable "last_n" {
  description = "Number of buckets to return, ending with the current one."
  type        = number
  default     = 7

  validation {
    condition     = var.last_n == null ? true : var.last_n >= 1
    error_message = "last_n must be at least 1."
  }
}

variable "pod_id" {
  description = "Restrict pod billing to one pod; null reports every pod."
  type        = string
  default     = null
}

variable "serverless_id" {
  description = "Restrict serverless billing to one endpoint; null reports every endpoint."
  type        = string
  default     = null
}

variable "network_volume_id" {
  description = "Restrict network volume billing to one volume; null reports every volume."
  type        = string
  default     = null
}

variable "cluster_id" {
  description = "Restrict cluster billing to one cluster; null reports every cluster."
  type        = string
  default     = null
}
