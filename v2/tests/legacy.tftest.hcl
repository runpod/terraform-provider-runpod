provider "runpod" {}

# The deprecated GraphQL and REST v1 shims, read-only half: the account
# identity read and the three 1.x billing data sources. Safe against any
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
    condition     = output.pod_time_billed_ms >= 0
    error_message = "pod time billed came back negative"
  }
  assert {
    condition     = output.network_volume_record_count >= 0 && output.network_volume_disk_gb >= 0
    error_message = "the network volume billing route returned an unusable record list"
  }
  assert {
    condition     = output.endpoint_record_count >= 0 && output.endpoint_spend >= 0
    error_message = "the endpoint billing route returned an unusable record list"
  }
}

run "filtered_by_gpu_type" {
  module {
    source = "../modules/legacy"
  }

  variables {
    bucket_size = "hour"
    gpu_type_id = "NVIDIA A100 80GB PCIe"
  }

  # The GPU type filter is the one thing REST v2 billing cannot do; every record
  # the filtered read returns has to be for that GPU type.
  assert {
    condition     = length(output.pod_gpu_types) == 0 || output.pod_gpu_types == ["NVIDIA A100 80GB PCIe"]
    error_message = "the gpu_type_id filter returned records for another GPU type"
  }
}
