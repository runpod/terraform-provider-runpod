resource "runpod_serverless_endpoint" "this" {
  name          = var.name
  image         = var.image
  template_id   = var.template_id
  endpoint_type = var.endpoint_type

  gpu_pools             = var.gpu_pools
  gpu_count             = var.gpu_count
  excluded_types        = var.excluded_types
  allowed_cuda_versions = var.allowed_cuda_versions
  min_cuda_version      = var.min_cuda_version
  data_center_ids       = var.data_center_ids

  flashboot     = var.flashboot
  scaling_type  = var.scaling_type
  scaling_value = var.scaling_value
  workers_min   = var.workers_min
  workers_max   = var.workers_max
  timeout       = var.timeout
  idle_timeout  = var.idle_timeout
  ports         = var.ports

  disk            = var.disk
  env             = var.env
  registry        = var.registry
  network_volumes = var.network_volumes

  # Deprecated, preserving the 1.x compute_type and gpu_type_priority
  # attributes. compute_type restates the GPU choice above and is derived from
  # what the v2 API reports; gpu_type_priority orders the types the scheduler
  # tries, goes to the REST v1 shim, and is never read back.
  compute_type      = var.compute_type
  gpu_type_priority = var.gpu_type_priority
}

# Operational views of the endpoint just created: its workers, its container
# builds (only an endpoint built from a Git repository has any), one build in
# detail when build_id names it, and its configuration releases with the
# current rollout.
data "runpod_endpoint_workers" "this" {
  endpoint_id = runpod_serverless_endpoint.this.id
}

data "runpod_endpoint_builds" "this" {
  endpoint_id = runpod_serverless_endpoint.this.id
}

data "runpod_endpoint_build" "this" {
  count = var.build_id == null ? 0 : 1

  endpoint_id = runpod_serverless_endpoint.this.id
  id          = var.build_id
}

data "runpod_endpoint_releases" "this" {
  endpoint_id = runpod_serverless_endpoint.this.id
}

# Every endpoint on the account, including the one above.
data "runpod_serverless_endpoints" "all" {
  depends_on = [runpod_serverless_endpoint.this]
}
