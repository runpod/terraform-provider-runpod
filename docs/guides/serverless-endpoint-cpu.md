---
page_title: "Serverless endpoint (CPU)"
subcategory: "Use-case modules"
description: |-
  Creates a CPU Serverless endpoint: cpu lists the flavor / vCPU pairs workers may be placed on instead of GPU pools.
---

# Serverless endpoint (CPU)

Creates a CPU Serverless endpoint: `cpu` lists the flavor / vCPU pairs
workers may be placed on instead of GPU pools. Scaled to zero workers by
default, so it costs nothing until requests arrive. Flavor identifiers and
their valid vCPU counts come from the CPU catalog (see
[catalog-discovery](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/catalog-discovery)).

Everything else works as in the GPU
[serverless-endpoint](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/serverless-endpoint)
module: `image` or `template_id`, queue or load-balancer routing, autoscaling
bounds, a `registry` credential for a private image, and `network_volumes`
for storage shared by every worker.

The deprecated inputs and outputs (`cpu_flavor_priority`, `users`, `version`,
`workers`) go through the REST v1 compatibility shim and exist for 1.x
configurations; `compute_type`, also deprecated, is derived from what REST v2
reports.

## Usage

```hcl
module "serverless_endpoint_cpu" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/serverless-endpoint-cpu?ref=v2.0.0-rc.3"

  name = "api"
  cpu = [{
    cpu_type_id = "cpu5c"
    vcpu_count  = 4
  }]
  workers_max = 3
}
```

## Resources and data sources

- [`runpod_serverless_endpoint`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/serverless_endpoint)

## Configuration

```terraform
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
  # what the v2 API reports; cpu_flavor_priority is rejected by REST v1 and
  # stays null.
  compute_type        = var.compute_type
  cpu_flavor_priority = var.cpu_flavor_priority
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0-rc.3/v2/modules/serverless-endpoint-cpu/README.md).
