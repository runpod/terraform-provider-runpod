provider "runpod" {}

# Rewrites the account's registered SSH keys. Every write replaces the whole
# set, so this run temporarily owns them all. remove_on_destroy stays false, so
# the key below is left registered after the test; delete it in the console if
# you do not want it. Nothing is billed.

variables {
  test_key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILXGDN/SclOozk1xsDztpmhGiKkkrfQB9SKoO8dSIQQZ terraform-provider-test"
}

run "register" {
  module {
    source = "../modules/account-ssh-keys"
  }

  variables {
    keys = [var.test_key]
  }

  assert {
    condition     = output.id == "account"
    error_message = "the singleton id is not the literal \"account\""
  }
  assert {
    condition     = output.key_count == 1
    error_message = "the account does not carry exactly the one key that was written"
  }
  assert {
    condition     = contains(output.keys, var.test_key)
    error_message = "the API did not echo back the key that was written"
  }
}
