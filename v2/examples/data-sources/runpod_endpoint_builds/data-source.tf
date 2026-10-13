data "runpod_endpoint_builds" "vllm" {
  endpoint_id = "ep_abc123"
}

output "build_statuses" {
  description = "Build state per build."
  value       = { for b in data.runpod_endpoint_builds.vllm.builds : b.id => b.status }
}
