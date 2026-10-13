# The account's SSH keys are one document the API replaces on every write, so
# declare this resource once and let it own every key on the account.
resource "runpod_account_ssh_keys" "this" {
  keys = [
    # A .pub file ends in a newline, which the API's key format does not allow.
    trimspace(file("~/.ssh/id_ed25519.pub")),
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILXGDN/SclOozk1xsDztpmhGiKkkrfQB9SKoO8dSIQQZ ci@example.com",
  ]

  # Destroy leaves the keys registered by default, so tearing down this
  # resource cannot cost you access to a running pod.
  remove_on_destroy = false
}
