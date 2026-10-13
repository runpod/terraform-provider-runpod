resource "runpod_account_ssh_keys" "this" {
  keys              = var.keys
  remove_on_destroy = var.remove_on_destroy
}
