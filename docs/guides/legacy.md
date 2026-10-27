---
page_title: "Legacy shims (deprecated)"
subcategory: "Use-case modules"
description: |-
  The deprecated 1.x surfaces the 2.0 provider still serves: machines and the account read over the GraphQL API, and the three 1.x billing data sources over REST v2.
---

# Legacy shims (deprecated)

The deprecated 1.x surfaces the 2.0 provider still serves: the host machine
resource and its two data sources and the account identity read (the GraphQL
`myself` query: only `id` and `email` are served) over the GraphQL API, because
REST v2 has no route for them, and the three 1.x billing data sources
(`runpod_billing_pod`, `runpod_billing_network_volume`,
`runpod_billing_endpoint`) in their 1.x record shape over the REST v2 billing
routes. The endpoint records' `time_billed_ms` comes from the GraphQL API's
serverless billing, and a `gpu_type_id` read of `runpod_billing_pod` from the
GraphQL GPU pod billing, per GPU type as REST v1 reported it. Otherwise the pod
and network volume records' `time_billed_ms` and `disk_space_billed_gb`, and
the pod records' `gpu_type_id` and `endpoint_id`, read null, because no API
reports them per pod or per volume. **Every part of
this module is deprecated**, warns on each plan, and is removed once REST v2
reports the same thing or the 1.x shape is dropped. New configurations use the
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
terraform apply -var bucket_size=day -var pod_id=pod_abc123
terraform apply -var create_machine=true -var machine_name=host-01
```

Point the GraphQL shims elsewhere with the provider's `graphql_url` /
`RUNPOD_GRAPHQL_URL` setting; the billing reads follow `base_url` /
`RUNPOD_API_BASE_URL`, like every other REST v2 call.

## Usage

```hcl
module "legacy" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/legacy?ref=v2.0.0"

  bucket_size = "day"
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
# Deprecated shims — the machines and the account read call the GraphQL API
# through the SDK's deprecated legacy client, because REST v2 has no route for
# them; the 1.x billing data sources read REST v2 in their 1.x record shape.
# Each is removed once REST v2 reports the same thing or the 1.x shape goes.

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

# The 1.x billing data sources, kept so a 1.x configuration still plans. They
# read the REST v2 billing routes and map each record back into the 1.x shape.
# The endpoint records' time_billed_ms comes from the GraphQL API's serverless
# billing. These reads set no gpu_type_id, so the pod and network volume
# records' time_billed_ms and disk_space_billed_gb, and the pod records'
# gpu_type_id and endpoint_id, read null: no API reports them per pod or per
# volume. New configurations use the billing-report module.
data "runpod_billing_pod" "this" {
  bucket_size = var.bucket_size
  end_time    = var.end_time
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
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0/v2/modules/legacy/README.md).
