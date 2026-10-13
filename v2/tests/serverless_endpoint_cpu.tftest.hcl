provider "runpod" {}

# A CPU endpoint: cpu replaces gpu_pools. Scaled to zero workers like the GPU
# endpoint, so it costs nothing until a request arrives.

run "create" {
  module {
    source = "../modules/serverless-endpoint-cpu"
  }

  variables {
    name          = "tf-test-cpu-endpoint"
    scaling_type  = "REQUEST_COUNT"
    scaling_value = 1
    workers_min   = 0
    workers_max   = 1
    cpu = [{
      cpu_type_id = "cpu5c"
      vcpu_count  = 4
    }]
    # compute_type restates the CPU choice. cpu_flavor_priority is left out:
    # the REST v1 PATCH it goes through rejects the field.
    compute_type = "CPU"
  }

  assert {
    condition     = output.id != ""
    error_message = "cpu endpoint id is empty"
  }
  assert {
    condition     = length(output.cpu) == 1
    error_message = "the cpu configuration was not applied"
  }
  assert {
    condition     = output.cpu[0].cpu_type_id == "cpu5c" && output.cpu[0].vcpu_count == 4
    error_message = "the cpu configuration did not round-trip"
  }
  assert {
    condition     = output.gpu_pools == null
    error_message = "a CPU endpoint must report no gpu pools"
  }
  assert {
    condition     = output.request_urls.run != "" && output.request_urls.run_sync != ""
    error_message = "a queue endpoint must report its job submission URLs"
  }
  assert {
    condition     = output.compute_type == "CPU"
    error_message = "a CPU endpoint must report compute_type CPU"
  }
  assert {
    condition     = output.version != null
    error_message = "the REST v1 shim did not report the endpoint's configuration version"
  }
  assert {
    condition     = length(output.users) == 1
    error_message = "the REST v1 shim must report the owning account as a one-element list"
  }
}
