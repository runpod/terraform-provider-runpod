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

output "cpu" {
  description = "CPU configurations workers are placed on."
  value       = runpod_serverless_endpoint.this.cpu
}

output "gpu_pools" {
  description = "GPU pools of the endpoint; always null for a CPU endpoint."
  value       = runpod_serverless_endpoint.this.gpu_pools
}

output "created_at" {
  description = "When the endpoint was created."
  value       = runpod_serverless_endpoint.this.created_at
}

output "request_urls" {
  description = "Invocation URLs for the endpoint: the job operations for a QUEUE endpoint, or base and health for a LOAD_BALANCER one."
  value       = runpod_serverless_endpoint.this.request_urls
}

# Deprecated, preserving the 1.x compute_type, users, version and workers
# attributes. compute_type is derived from the v2 response, users is read back
# from the GraphQL shim, and version and workers from the REST v2 workers
# listing.
output "compute_type" {
  description = "Deprecated. Worker compute family; CPU for this module."
  value       = runpod_serverless_endpoint.this.compute_type
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
  description = "Deprecated. Workers allocated at the moment the endpoint was read."
  value       = runpod_serverless_endpoint.this.workers
}
