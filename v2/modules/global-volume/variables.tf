variable "name" {
  description = "Volume name, 1 to 191 characters with no leading or trailing whitespace. Renamed in place."
  type        = string
  default     = "example-global-volume"

  validation {
    condition     = length(var.name) >= 1 && length(var.name) <= 191 && trimspace(var.name) == var.name
    error_message = "name must be 1 to 191 characters with no leading or trailing whitespace."
  }
}
