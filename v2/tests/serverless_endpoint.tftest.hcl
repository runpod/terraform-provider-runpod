provider "runpod" {}

# Creates a real endpoint scaled to zero workers, reads back its workers,
# builds and releases, updates it in place, and destroys it afterwards. The
# build reads only return anything for an endpoint built from a Git
# repository, so the build assertions check shape rather than content.

run "create" {
  module {
    source = "../modules/serverless-endpoint"
  }

  variables {
    name        = "tf-test-endpoint"
    workers_min = 0
    workers_max = 1
    timeout     = 300000
    flashboot   = "FLASHBOOT"
    # Deprecated: compute_type restates the GPU choice and is derived from the
    # v2 read. gpu_type_priority is not set: REST v1 no longer accepts it.
    compute_type = "GPU"
  }

  assert {
    condition     = output.id != ""
    error_message = "endpoint id is empty"
  }
  assert {
    condition     = output.timeout == 300000
    error_message = "create-time timeout was not honored"
  }
  assert {
    condition     = output.flashboot == "FLASHBOOT"
    error_message = "create-time flashboot was not honored"
  }
  assert {
    condition     = output.cpu == null
    error_message = "a GPU endpoint must report no cpu config"
  }
  assert {
    condition     = output.request_urls.run != "" && output.request_urls.run_sync != ""
    error_message = "a queue endpoint must report its job submission URLs"
  }
  assert {
    condition     = output.request_urls.base == null
    error_message = "a queue endpoint has no load-balancer base URL"
  }
  assert {
    condition     = contains(output.endpoint_ids, output.id)
    error_message = "the new endpoint is missing from the endpoints listing"
  }
  assert {
    condition     = alltrue([for id in output.endpoint_ids : id != ""])
    error_message = "endpoints listing returned an entry with an empty id"
  }
  # The operational reads: workers, container builds, and the release rollout.
  assert {
    condition     = output.worker_summary.total >= 0
    error_message = "the worker summary came back with a negative total"
  }
  assert {
    condition     = alltrue([for id in output.worker_ids : id != ""])
    error_message = "the worker listing returned an entry with an empty id"
  }
  assert {
    condition     = output.build_statuses != null
    error_message = "the build listing returned no status map at all"
  }
  assert {
    condition     = alltrue([for status in values(output.build_statuses) : status != ""])
    error_message = "a build came back with an empty status"
  }
  assert {
    condition     = output.rollout.workers_total >= output.rollout.workers_on_latest
    error_message = "the rollout reports more workers on the latest version than exist"
  }
  assert {
    condition     = output.rollout.percent_on_latest >= 0 && output.rollout.percent_on_latest <= 100
    error_message = "the rollout percentage is outside 0-100"
  }
  assert {
    condition     = output.compute_type == "GPU"
    error_message = "compute_type did not round-trip"
  }
  assert {
    condition     = output.version != null
    error_message = "the REST v1 shim did not report the endpoint's configuration version"
  }
  assert {
    condition     = length(output.users) == 1
    error_message = "the REST v1 shim must report the owning account as a one-element list"
  }
  # Scaled to zero workers, so the list is usually null: the provider reports
  # an endpoint with no workers as null, not as an empty list. A worker the API
  # has allocated anyway must still carry an id.
  assert {
    condition     = alltrue([for w in(output.workers == null ? [] : output.workers) : w.id != ""])
    error_message = "the REST v1 shim reported a worker with an empty id"
  }
}

run "update" {
  module {
    source = "../modules/serverless-endpoint"
  }

  variables {
    name             = "tf-test-endpoint-renamed"
    workers_min      = 0
    workers_max      = 2
    timeout          = 600000
    flashboot        = "OFF"
    min_cuda_version = "12.1"
    compute_type     = "GPU"
  }

  assert {
    condition     = output.name == "tf-test-endpoint-renamed"
    error_message = "in-place rename did not round-trip"
  }
  assert {
    condition     = output.timeout == 600000
    error_message = "in-place timeout update did not round-trip"
  }
  assert {
    condition     = output.flashboot == "OFF"
    error_message = "in-place flashboot update did not round-trip"
  }
  assert {
    condition     = output.min_cuda_version == "12.1"
    error_message = "in-place CUDA floor update did not round-trip"
  }
  assert {
    condition     = output.version != null
    error_message = "the REST v1 shim did not report the endpoint's configuration version after the update"
  }
}
