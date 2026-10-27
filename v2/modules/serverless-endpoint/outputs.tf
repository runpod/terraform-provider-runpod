output "id" {
  description = "Endpoint identifier."
  value       = runpod_serverless_endpoint.this.id
}

output "name" {
  description = "Endpoint name."
  value       = runpod_serverless_endpoint.this.name
}

output "timeout" {
  description = "Effective per-request timeout in milliseconds."
  value       = runpod_serverless_endpoint.this.timeout
}

output "image" {
  description = "Image the workers run, whether set directly or resolved from a template."
  value       = runpod_serverless_endpoint.this.image
}

output "flashboot" {
  description = "Effective FlashBoot mode."
  value       = runpod_serverless_endpoint.this.flashboot
}

output "gpu_pools" {
  description = "GPU pools workers are placed on."
  value       = runpod_serverless_endpoint.this.gpu_pools
}

output "min_cuda_version" {
  description = "Effective CUDA floor for worker placement."
  value       = runpod_serverless_endpoint.this.min_cuda_version
}

output "cpu" {
  description = "CPU configurations of the endpoint; always null for a GPU endpoint."
  value       = runpod_serverless_endpoint.this.cpu
}

output "created_at" {
  description = "When the endpoint was created."
  value       = runpod_serverless_endpoint.this.created_at
}

output "request_urls" {
  description = "Invocation URLs for the endpoint: the job operations for a QUEUE endpoint, or base and health for a LOAD_BALANCER one."
  value       = runpod_serverless_endpoint.this.request_urls
}

output "worker_summary" {
  description = "Worker counts by status, from runpod_endpoint_workers."
  value       = data.runpod_endpoint_workers.this.summary
}

output "worker_ids" {
  description = "Identifiers of the endpoint's current workers; the input for the logs module."
  value       = [for w in data.runpod_endpoint_workers.this.workers : w.id]
}

output "stale_worker_ids" {
  description = "Workers still running an older configuration version."
  value       = [for w in data.runpod_endpoint_workers.this.workers : w.id if w.is_stale]
}

output "build_statuses" {
  description = "Build state per container build, newest first as the API returns them. Empty for an endpoint deployed from a prebuilt image."
  value       = { for b in data.runpod_endpoint_builds.this.builds : b.id => b.status }
}

output "build_image" {
  description = "Image the build named by build_id produced; null when build_id is unset."
  value       = try(one(data.runpod_endpoint_build.this).image_name, null)
}

output "rollout" {
  description = "How far the current configuration version has reached the endpoint's workers."
  value       = data.runpod_endpoint_releases.this.rollout
}

output "release_versions" {
  description = "Configuration version per release."
  value       = [for r in data.runpod_endpoint_releases.this.releases : r.version]
}

output "endpoint_ids" {
  description = "Identifiers of every Serverless endpoint on the account, including this one."
  value       = [for e in data.runpod_serverless_endpoints.all.endpoints : e.id]
}

# Deprecated, preserving the 1.x compute_type, template_version, users, version
# and workers attributes. compute_type is derived from the v2 response,
# template_version has no source in any API, users is read back from the
# GraphQL shim, and version and workers from the REST v2 workers listing.
output "compute_type" {
  description = "Deprecated. Worker compute family; GPU for this module."
  value       = runpod_serverless_endpoint.this.compute_type
}

output "template_version" {
  description = "Deprecated. Always null: neither REST v2 nor GraphQL reports the version of the template the endpoint was built from."
  value       = runpod_serverless_endpoint.this.template_version
}

output "users" {
  description = "Deprecated. Account that owns the endpoint, as a one-element list."
  value       = runpod_serverless_endpoint.this.users
}

output "version" {
  description = "Deprecated. The endpoint's configuration version."
  value       = runpod_serverless_endpoint.this.version
}

output "workers" {
  description = "Deprecated. Workers allocated at the moment the endpoint was read; worker_ids is the supported view of the same thing."
  value       = runpod_serverless_endpoint.this.workers
}
