---
page_title: "Serverless endpoint (GPU)"
subcategory: "Serverless"
description: |-
  Creates a GPU Serverless endpoint from an image or a template, scaled to zero workers by default, and reads back its operational state.
---

# Serverless endpoint (GPU)

Creates a GPU Serverless endpoint from an image or a template, scaled to zero
workers by default so it costs nothing until requests arrive, and reads back
its operational state: current workers, container builds, one build in
detail, and the configuration releases with the current rollout. For CPU
workers use
[serverless-endpoint-cpu](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/serverless-endpoint-cpu).

`image` or `template_id`: an image builds the endpoint directly; a template
(see
[template](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/template))
supplies the container settings instead. Set `ports` together with
`endpoint_type = "LOAD_BALANCER"` and `scaling_type = "REQUEST_COUNT"` to
route requests straight to the worker's own HTTP paths instead of through the
managed queue; `request_urls` then carries the base and health URLs rather
than the job operations.

On the GPU side, `gpu_pools` selects the pool (the `pool` field in the GPU
catalog, see
[catalog-discovery](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/catalog-discovery)),
`excluded_types` subtracts specific cards, `allowed_cuda_versions` /
`min_cuda_version` constrain worker placement, and `flashboot` selects the
cold-start acceleration mode. A private worker image needs a `registry`
credential (see
[registry](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/registry));
`network_volumes` mounts volumes from
[network-volume](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/network-volume)
into every worker.

The build reads only return anything for an endpoint built from a Git
repository; `build_id` is optional and skips the by-id read when null.
Worker ids change as the endpoint scales; `worker_ids` is the input the
[logs](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/logs)
module takes for a worker's log stream.

The deprecated inputs and outputs (`gpu_type_priority`, `template_version`,
`users`, `version`, `workers`) go through the REST v1 compatibility shim and
exist for 1.x configurations; `compute_type`, also deprecated, is derived from
what REST v2 reports.

## Usage

```hcl
module "serverless_endpoint" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/serverless-endpoint?ref=v2.0.0"

  name        = "inference"
  gpu_pools   = ["ADA_24"]
  workers_max = 3
}
```

## Resources and data sources

- [`runpod_serverless_endpoint`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/serverless_endpoint)
- [`runpod_endpoint_workers`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_workers)
- [`runpod_endpoint_builds`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_builds)
- [`runpod_endpoint_build`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_build)
- [`runpod_endpoint_releases`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_releases)
- [`runpod_serverless_endpoints`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/serverless_endpoints)

## Configuration

```terraform
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
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0/v2/modules/serverless-endpoint/README.md).
