---
page_title: "Legacy shims (deprecated)"
subcategory: "Use-case modules"
description: |-
  Everything the 2.0 provider still serves from the GraphQL and REST v1 APIs because REST v2 has no route for it: machines, the account read, and the three 1.x billing data sources.
---

# Legacy shims (deprecated)

Everything the 2.0 provider still serves from the GraphQL and REST v1 APIs
because REST v2 has no route for it: the host machine resource and its two
data sources, the account identity read (the GraphQL `myself` query: only
`id` and `email` are served, REST v1 `/user` is gone), and the three 1.x billing data
sources (`runpod_billing_pod`, `runpod_billing_network_volume`,
`runpod_billing_endpoint`) with their per-record `gpu_type_id`,
`disk_space_billed_gb`, `endpoint_id` and `time_billed_ms` fields. **Every
part of this module is deprecated**, warns on each plan, and is removed once
REST v2 reports the same thing. The billing data sources read REST v1, which is
retired on 2026-11-15; from that date the module's default plan fails until the
billing reads are removed. New configurations use the
[billing-report](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/billing-report)
module and never register machines from Terraform.

The
[migration guide](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/migrating-from-1-x)
lists every shim with the 1.x attribute or data source it preserves and what
replaces it.

By default the module is read-only and safe against any account: the
account read and the billing reads. `create_machine = true` additionally
registers a host machine, which needs a host account.

```bash
terraform apply -var bucket_size=day -var gpu_type_id="NVIDIA A100 80GB PCIe"
terraform apply -var create_machine=true -var machine_name=host-01
```

Point the shims elsewhere with the provider's `graphql_url` /
`RUNPOD_GRAPHQL_URL` (machines and the account read) and `rest_v1_url` /
`RUNPOD_REST_V1_URL` (billing) settings.

## Usage

```hcl
module "legacy" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/legacy?ref=v2.0.0-rc.3"

  bucket_size = "day"
  gpu_type_id = "NVIDIA A100 80GB PCIe"
}
```

## Resources and data sources

- [`runpod_machine`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/machine)
- [`runpod_machine`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/machine) (data source)
- [`runpod_machines`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/machines)
- [`runpod_user`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/user)
- [`runpod_billing_pod`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/billing_pod)
- [`runpod_billing_network_volume`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/billing_network_volume)
- [`runpod_billing_endpoint`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/billing_endpoint)

## Configuration

```terraform
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
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0-rc.3/v2/modules/legacy/README.md).
