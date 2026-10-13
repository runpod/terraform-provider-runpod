variable "keys" {
  description = <<-EOT
    The complete set of SSH public keys to register on the account, one
    authorized_keys-style line each. No default on purpose: every write
    replaces the whole set, so an accidental apply with an empty list would
    unregister the keys you already have. Pass your own, e.g.
    -var 'keys=["ssh-ed25519 AAAA... me@example.com"]'.
  EOT
  type        = set(string)
}

variable "remove_on_destroy" {
  description = "Whether destroying the resource also clears the account's keys. False leaves them registered."
  type        = bool
  default     = false
}
