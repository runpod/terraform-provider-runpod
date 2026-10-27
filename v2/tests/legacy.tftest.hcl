provider "runpod" {}

# The deprecated shims, read-only half: the account identity read over
# GraphQL and the three 1.x billing data sources over REST v2. Safe against any
# account: the amounts depend on what the account has run, so the assertions
# check the shape of the response rather than a value. create_machine stays
# false here; the machine surface is tests/legacy_machine.tftest.hcl.

run "account_and_billing" {
  module {
    source = "../modules/legacy"
  }

  variables {
    bucket_size = "day"
  }

  assert {
    condition     = output.account_id != ""
    error_message = "the account response carried no id"
  }
  assert {
    condition     = output.machine_id == null && output.machine_gpu_total == null
    error_message = "a machine was registered without create_machine"
  }
  assert {
    condition     = output.pod_record_count >= 0
    error_message = "the pod billing route returned no record list"
  }
  assert {
    condition     = output.pod_spend >= 0
    error_message = "pod spend came back negative"
  }
  assert {
    condition     = output.network_volume_record_count >= 0
    error_message = "the network volume billing route returned no record list"
  }
  assert {
    condition     = output.endpoint_record_count >= 0 && output.endpoint_spend >= 0
    error_message = "the endpoint billing route returned an unusable record list"
  }
}
