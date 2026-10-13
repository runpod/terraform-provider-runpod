# The API streams pod logs live. The read collects the backfill (50 lines
# here) and returns; read_timeout_seconds caps how long it waits for them.
data "runpod_pod_logs" "training" {
  pod_id               = "pod_abc123"
  source               = "container"
  tail                 = 50
  read_timeout_seconds = 10
}

output "log_text" {
  description = "The pod's recent container output as one string."
  value       = join("\n", data.runpod_pod_logs.training.lines)
}

output "resume_cursor" {
  description = "Cursor of the last event read; pass it back as last_event_id to continue from here."
  value       = data.runpod_pod_logs.training.last_id
}
