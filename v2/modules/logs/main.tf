# Bounded reads of the two live log streams. Each is a real event stream:
# the read returns the `tail` backfill and stops, or stops after
# read_timeout_seconds with whatever arrived. Both are optional so one module
# call can read either or both.
data "runpod_pod_logs" "this" {
  count = var.pod_id == null ? 0 : 1

  pod_id               = var.pod_id
  source               = var.log_source
  tail                 = var.tail
  read_timeout_seconds = var.read_timeout_seconds
}

data "runpod_endpoint_worker_logs" "this" {
  count = var.worker_id == null ? 0 : 1

  endpoint_id          = var.endpoint_id
  worker_id            = var.worker_id
  source               = var.log_source
  tail                 = var.tail
  read_timeout_seconds = var.read_timeout_seconds

  lifecycle {
    precondition {
      condition     = var.endpoint_id != null
      error_message = "worker_id needs the endpoint_id the worker belongs to."
    }
  }
}
