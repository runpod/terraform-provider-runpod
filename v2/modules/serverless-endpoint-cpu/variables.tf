variable "name" {
  description = "Endpoint name."
  type        = string
  default     = "example-cpu-endpoint"
}

variable "image" {
  description = "Docker image reference for workers. Null when template_id supplies one."
  type        = string
  default     = "runpod/mock-worker:dev"
}

variable "template_id" {
  description = "Serverless template to base the endpoint on (see the template module). Null builds it from image alone."
  type        = string
  default     = null
}

variable "endpoint_type" {
  description = "Request routing: QUEUE or LOAD_BALANCER."
  type        = string
  default     = "QUEUE"

  validation {
    condition     = var.endpoint_type == null ? true : contains(["QUEUE", "LOAD_BALANCER"], var.endpoint_type)
    error_message = "endpoint_type must be QUEUE or LOAD_BALANCER."
  }
}

variable "cpu" {
  description = "CPU configurations workers may be placed on: a flavor from the CPU catalog (see the catalog-discovery module) and a vCPU count valid for it."
  type = list(object({
    cpu_type_id = string
    vcpu_count  = number
  }))
  default = [{
    cpu_type_id = "cpu5c"
    vcpu_count  = 4
  }]
}

variable "data_center_ids" {
  description = "Data centers workers may be placed in. Null lets the scheduler choose; a network volume requires the workers to land in the volume's data center."
  type        = list(string)
  default     = null
}

variable "scaling_type" {
  description = "Autoscaling signal: QUEUE_DELAY or REQUEST_COUNT."
  type        = string
  default     = "QUEUE_DELAY"

  validation {
    condition     = var.scaling_type == null ? true : contains(["QUEUE_DELAY", "REQUEST_COUNT"], var.scaling_type)
    error_message = "scaling_type must be QUEUE_DELAY or REQUEST_COUNT."
  }
}

variable "scaling_value" {
  description = "Scaler threshold: seconds of queue delay, or in-flight request count."
  type        = number
  default     = 4
}

variable "workers_min" {
  description = "Minimum number of workers. Zero costs nothing until requests arrive."
  type        = number
  default     = 0

  validation {
    condition     = var.workers_min == null ? true : var.workers_min >= 0
    error_message = "workers_min cannot be negative."
  }
}

variable "workers_max" {
  description = "Maximum number of workers."
  type        = number
  default     = 1

  validation {
    condition     = var.workers_max == null ? true : var.workers_max >= 0
    error_message = "workers_max cannot be negative."
  }
}

variable "timeout" {
  description = "Per-request execution timeout in milliseconds. Null uses the API default."
  type        = number
  default     = null
}

variable "idle_timeout" {
  description = "Seconds an idle worker stays up before scaling down, 1 to 3600. Null uses the API default."
  type        = number
  default     = null

  validation {
    condition     = var.idle_timeout == null ? true : (var.idle_timeout >= 1 && var.idle_timeout <= 3600)
    error_message = "idle_timeout must be between 1 and 3600 seconds."
  }
}

variable "ports" {
  description = "Worker HTTP ports a LOAD_BALANCER endpoint routes to, e.g. [\"8000/http\"]. Null for a QUEUE endpoint, which takes requests through the managed queue."
  type        = list(string)
  default     = null
}

variable "disk" {
  description = "Worker container disk in GB. Null uses the API default."
  type        = number
  default     = null

  validation {
    condition     = var.disk == null ? true : var.disk > 0
    error_message = "disk must be greater than 0 GB."
  }
}

variable "env" {
  description = "Environment variables set on every worker."
  type        = map(string)
  default     = {}
}

variable "registry" {
  description = "Registry credential ID (see the registry module) for pulling a private worker image. Null for public images."
  type        = string
  default     = null
}

variable "network_volumes" {
  description = "Network volume IDs (see the network-volume module) mounted into every worker. Null attaches none."
  type        = list(string)
  default     = null
}

variable "compute_type" {
  description = "Deprecated. Worker compute family, preserving the 1.x compute_type attribute. Null derives it from cpu; a value other than CPU is rejected here."
  type        = string
  default     = null

  validation {
    condition     = var.compute_type == null ? true : var.compute_type == "CPU"
    error_message = "compute_type on a CPU endpoint must be CPU; the provider rejects a value that contradicts cpu."
  }
}

variable "cpu_flavor_priority" {
  description = "Deprecated and not applied: neither REST v2 nor GraphQL has a setting for the order of the configured CPU flavors, so a value is kept in state, never sent, and warned about at plan time. Leave null."
  type        = string
  default     = null
}
