provider "runpod" {}

# The deprecated GraphQL machine surface, opt-in half of the legacy module:
# registers a real host machine on the account and reads it back through both
# machine data sources. Needs a host account: a plain account cannot add
# machines, so this file cannot pass against an arbitrary account.

run "create" {
  module {
    source = "../modules/legacy"
  }

  variables {
    create_machine = true
    machine_name   = "tf-test-machine"
  }

  assert {
    condition     = output.machine_id != ""
    error_message = "machine id is empty"
  }
  assert {
    condition     = output.machine_name == "tf-test-machine"
    error_message = "machine name does not match the requested name"
  }
  assert {
    condition     = output.machine_gpu_total != null
    error_message = "the machine data source returned no gpu_total"
  }
  assert {
    condition     = contains(keys(output.machine_names), output.machine_id)
    error_message = "the new machine is missing from the machines listing"
  }
}
