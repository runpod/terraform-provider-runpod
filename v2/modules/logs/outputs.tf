output "pod_log_lines" {
  description = "Log lines the pod's stream delivered; null when pod_id is unset."
  value       = try(one(data.runpod_pod_logs.this).lines, null)
}

output "pod_log_last_id" {
  description = "Cursor of the last pod log event read, for a later read's last_event_id; null when pod_id is unset."
  value       = try(one(data.runpod_pod_logs.this).last_id, null)
}

output "worker_log_lines" {
  description = "Log lines the worker's stream delivered; null when worker_id is unset."
  value       = try(one(data.runpod_endpoint_worker_logs.this).lines, null)
}

output "worker_log_last_id" {
  description = "Cursor of the last worker log event read, for a later read's last_event_id; null when worker_id is unset."
  value       = try(one(data.runpod_endpoint_worker_logs.this).last_id, null)
}
