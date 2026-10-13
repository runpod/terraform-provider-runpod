variable "pod_id" {
  description = "Pod whose log stream to read. Null skips the pod read."
  type        = string
  default     = null
}

variable "endpoint_id" {
  description = "Serverless endpoint the worker belongs to. Required with worker_id."
  type        = string
  default     = null
}

variable "worker_id" {
  description = "Worker whose log stream to read, from the serverless-endpoint module's worker_ids. Null skips the worker read. Workers come and go with load, so the id has to be current."
  type        = string
  default     = null
}

# Not named "source": that is reserved inside module blocks, so a variable
# called source cannot be declared at all.
variable "log_source" {
  description = "Log source to read: \"container\" for the container's stdout and stderr, \"system\" for the host lifecycle stream, or null for both."
  type        = string
  default     = "container"

  validation {
    condition     = var.log_source == null ? true : contains(["container", "system"], var.log_source)
    error_message = "log_source must be \"container\", \"system\", or null for both."
  }
}

variable "tail" {
  description = "Historical lines to backfill from the stream, 0 to 5000. The read returns as soon as that many events arrived or the stream went quiet."
  type        = number
  default     = 20

  validation {
    condition     = var.tail == null ? true : (var.tail >= 0 && var.tail <= 5000)
    error_message = "tail must be between 0 and 5000 lines."
  }
}

variable "read_timeout_seconds" {
  description = "Hard bound on each read, for a stream that keeps producing. Null uses the provider default of 10; 60 is the ceiling, since the provider's HTTP client closes the connection there."
  type        = number
  default     = null

  validation {
    condition     = var.read_timeout_seconds == null ? true : (var.read_timeout_seconds >= 1 && var.read_timeout_seconds <= 60)
    error_message = "read_timeout_seconds must be between 1 and 60."
  }
}
