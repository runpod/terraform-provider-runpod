resource "runpod_serverless_endpoint" "this" {
  name          = var.name
  image         = var.image
  template_id   = var.template_id
  endpoint_type = var.endpoint_type

  # CPU workers: cpu configurations instead of gpu_pools.
  cpu             = var.cpu
  data_center_ids = var.data_center_ids

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

  # Deprecated, preserving the 1.x compute_type and cpu_flavor_priority
  # attributes. compute_type restates the CPU choice above and is derived from
  # what the v2 API reports; cpu_flavor_priority has no setting in any API, so
  # a value is kept in state, never sent, and warned about at plan time.
  compute_type        = var.compute_type
  cpu_flavor_priority = var.cpu_flavor_priority
}
