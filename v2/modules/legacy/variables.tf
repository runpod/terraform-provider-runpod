variable "create_machine" {
  description = "Register a host machine through the deprecated GraphQL shim. Needs a host account; a plain account cannot add machines."
  type        = bool
  default     = false
}

variable "list_machines" {
  description = "List the account's host machines through the deprecated GraphQL shim. Needs a host account; creating a machine lists them too."
  type        = bool
  default     = false
}

variable "machine_name" {
  description = "Machine name, the only attribute the deprecated machineAdd mutation accepts."
  type        = string
  default     = "example-machine"
}

variable "bucket_size" {
  description = "Length of each 1.x billing bucket: hour, day, week, month, or year."
  type        = string
  default     = "day"

  validation {
    condition     = var.bucket_size == null ? true : contains(["hour", "day", "week", "month", "year"], var.bucket_size)
    error_message = "bucket_size must be hour, day, week, month, or year."
  }
}

variable "end_time" {
  description = "End of the reported 1.x billing window: an RFC 3339 timestamp, or a date or a datetime without a zone, read as UTC; null ends it at the current time. The window is the 30 days before it."
  type        = string
  default     = null
}

variable "pod_id" {
  description = "Restrict 1.x pod billing to one pod; null reports every pod."
  type        = string
  default     = null
}

variable "network_volume_id" {
  description = "Restrict 1.x network volume billing to one volume; null reports every volume."
  type        = string
  default     = null
}

variable "endpoint_id" {
  description = "Restrict 1.x Serverless endpoint billing to one endpoint; null reports every endpoint."
  type        = string
  default     = null
}
