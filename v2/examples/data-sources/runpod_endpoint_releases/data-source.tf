data "runpod_endpoint_releases" "vllm" {
  endpoint_id = "ep_abc123"
}

output "rollout_in_progress" {
  description = "Whether any worker still runs an older version."
  value       = data.runpod_endpoint_releases.vllm.rollout.in_progress
}

output "latest_release_changes" {
  description = "Configuration fields the newest release changed."
  value       = [for d in data.runpod_endpoint_releases.vllm.releases[0].diff : d.field]
}
