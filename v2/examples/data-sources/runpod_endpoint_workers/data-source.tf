data "runpod_endpoint_workers" "vllm" {
  endpoint_id = "ep_abc123"
}

output "running_workers" {
  description = "How many workers are handling requests."
  value       = data.runpod_endpoint_workers.vllm.summary.running
}

output "stale_worker_ids" {
  description = "Workers still running an older configuration version."
  value       = [for w in data.runpod_endpoint_workers.vllm.workers : w.id if w.is_stale]
}
