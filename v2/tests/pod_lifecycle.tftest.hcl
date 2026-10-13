provider "runpod" {}

# The GPU the pod tests create. A run block can only read variables declared in
# its own test file, so it is declared here; TF_VAR_gpu_type_id overrides it.
variables {
  gpu_type_id = "NVIDIA L40S"
}

# Creates a real (billed) pod, restarts it through runpod_pod_action, reads the
# pod back by id, then lets terraform test destroy the pod. The action resource
# itself costs nothing and its own destroy calls no API.

run "create_pod" {
  module {
    source = "../modules/gpu-pod"
  }

  variables {
    gpu_type_id = var.gpu_type_id
    name        = "tf-test-pod-action"
    cloud       = "SECURE"
    disk        = 20
  }

  assert {
    condition     = output.id != ""
    error_message = "pod id is empty"
  }
}

run "restart" {
  module {
    source = "../modules/pod-lifecycle"
  }

  variables {
    pod_id = run.create_pod.id
    action = "restart"
    triggers = {
      revision = "1"
    }
  }

  assert {
    condition     = output.id == run.create_pod.id
    error_message = "the action did not run against the pod it was pointed at"
  }
  assert {
    condition     = output.action == "restart"
    error_message = "the action recorded in state is not the one that was requested"
  }
  # The by-id read after the transition, and the account-wide pod listing.
  assert {
    condition     = output.pod_status != ""
    error_message = "pod lookup returned an empty status"
  }
  assert {
    condition     = output.pod_cost_per_hour >= 0
    error_message = "pod cost came back negative"
  }
  assert {
    condition     = contains(output.pod_ids, run.create_pod.id)
    error_message = "the restarted pod is missing from the pods listing"
  }
  assert {
    condition     = alltrue([for id in output.pod_ids : id != ""])
    error_message = "pods listing returned an entry with an empty id"
  }
}
