# Deprecated shims — everything here calls the GraphQL or REST v1 API through
# the SDK's deprecated legacy client, because REST v2 has no route for it.
# Each is removed once REST v2 reports the same thing.

# Host machines have no REST v2 route. Registering one means adding hardware
# as a Runpod host, so the resource is opt-in and needs a host account.
resource "runpod_machine" "this" {
  count = var.create_machine ? 1 : 0

  name = var.machine_name
}

data "runpod_machine" "this" {
  count = var.create_machine ? 1 : 0

  id = runpod_machine.this[0].id
}

# Listing machines needs a host account too: a plain account gets Unauthorized.
data "runpod_machines" "all" {
  count = var.list_machines || var.create_machine ? 1 : 0

  depends_on = [runpod_machine.this]
}

# The account the API key belongs to: the GraphQL myself query, there is no v2
# identity route and REST v1 /user is gone. Only id and email are served.
data "runpod_user" "this" {}

# The 1.x billing data sources over the REST v1 billing routes, kept for the
# per-record fields REST v2 billing does not report: gpu_type_id,
# disk_space_billed_gb, endpoint_id and time_billed_ms. New configurations use
# the billing-report module.
data "runpod_billing_pod" "this" {
  bucket_size = var.bucket_size
  end_time    = var.end_time
  gpu_type_id = var.gpu_type_id
  pod_id      = var.pod_id
}

data "runpod_billing_network_volume" "this" {
  bucket_size       = var.bucket_size
  end_time          = var.end_time
  network_volume_id = var.network_volume_id
}

data "runpod_billing_endpoint" "this" {
  bucket_size = var.bucket_size
  end_time    = var.end_time
  endpoint_id = var.endpoint_id
}
