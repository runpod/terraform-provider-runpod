variable "pod_id" {
  description = "Pod to act on. No default: the action runs against a real pod as soon as you apply."
  type        = string
}

variable "action" {
  description = "Transition to run: start, stop, restart, or terminate."
  type        = string
  default     = "restart"

  validation {
    condition     = var.action == null ? true : contains(["start", "stop", "restart", "terminate"], var.action)
    error_message = "action must be start, stop, restart, or terminate."
  }
}

variable "triggers" {
  description = "Values that re-run the action when any of them changes, e.g. { image = var.image }."
  type        = map(string)
  default     = {}
}

variable "include_cluster_pods" {
  description = "Include pods that belong to a cluster in the account listing. Null uses the API default, which leaves them out."
  type        = bool
  default     = null
}
