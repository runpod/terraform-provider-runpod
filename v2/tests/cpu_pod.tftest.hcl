provider "runpod" {}

# A CPU pod: cpu_type_id and vcpu_count replace gpu_type_id. Billed like the
# GPU pod, at CPU rates; terraform test destroys it afterwards.

run "create" {
  module {
    source = "../modules/cpu-pod"
  }

  variables {
    name          = "tf-test-cpu-pod"
    cpu_type_id   = "cpu5c"
    vcpu_count    = 4
    disk          = 20
    start_jupyter = false
  }

  assert {
    condition     = output.id != ""
    error_message = "cpu pod id is empty"
  }
  assert {
    condition     = output.status != ""
    error_message = "cpu pod status is empty"
  }
  assert {
    condition     = output.cpu_type_id == "cpu5c"
    error_message = "cpu_type_id did not round-trip"
  }
  assert {
    condition     = output.vcpu_count == 4
    error_message = "vcpu_count did not round-trip"
  }
  assert {
    condition     = output.gpu_type_id == null
    error_message = "a CPU pod must report no gpu config"
  }
  assert {
    condition     = output.network_volumes == null
    error_message = "a pod with no mount configured must report none"
  }
  assert {
    condition     = output.memory_in_gb > 0
    error_message = "the pod's allocated system RAM was not read back"
  }
  # The deprecated REST v1 shim, as on the GPU pod: machine_id only.
  assert {
    condition     = output.machine_id != null
    error_message = "the REST v1 shim did not report the pod's host machine"
  }
}
