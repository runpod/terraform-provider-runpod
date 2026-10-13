provider "runpod" {}

# The GPU the pod tests create. A run block can only read variables declared in
# its own test file, so it is declared here. A file-level variables block would
# outrank TF_VAR_gpu_type_id; a variable block's default does not.
variable "gpu_type_id" {
  type    = string
  default = "NVIDIA L40S"
}

# Creates a real (billed) pod and tails its container log stream. Reading is
# free; the pod costs the same per-hour rate as the gpu-pod test for as long as
# the run takes. Only the pod half runs: worker ids change as an endpoint
# scales, so no worker id can be named up front.

run "create_pod" {
  module {
    source = "../modules/gpu-pod"
  }

  variables {
    gpu_type_id = var.gpu_type_id
    name        = "tf-test-pod-logs"
    cloud       = "SECURE"
    disk        = 20
  }

  assert {
    condition     = output.id != ""
    error_message = "pod id is empty"
  }
}

run "tail_pod_log" {
  module {
    source = "../modules/logs"
  }

  variables {
    pod_id = run.create_pod.id
    tail   = 5
  }

  assert {
    condition     = output.pod_log_lines != null
    error_message = "the pod log read returned no line list at all"
  }
  assert {
    condition     = output.worker_log_lines == null
    error_message = "the worker log read ran without a worker_id"
  }
}
