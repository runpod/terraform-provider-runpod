# The API streams worker logs live. The read collects the backfill (50 lines
# here) and returns; read_timeout_seconds caps how long it waits for them.
data "runpod_endpoint_worker_logs" "vllm" {
  endpoint_id          = "ep_abc123"
  worker_id            = "worker_abc"
  tail                 = 50
  read_timeout_seconds = 10
}

output "worker_log_text" {
  description = "The worker's recent output as one string."
  value       = join("\n", data.runpod_endpoint_worker_logs.vllm.lines)
}

output "resume_cursor" {
  description = "Cursor of the last event read; pass it back as last_event_id to continue from here."
  value       = data.runpod_endpoint_worker_logs.vllm.last_id
}
