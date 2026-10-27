---
page_title: "Catalog discovery"
subcategory: "Use-case modules"
description: |-
  Reads the catalogs every other module picks its identifiers from: GPU types, CPU flavors, data centers, and one slice of the publicly shared templates.
---

# Catalog discovery

Reads the catalogs every other module picks its identifiers from: the GPU
types with their Serverless pools and prices, the CPU flavors with their
vCPU bounds, the data centers with their capabilities, and one slice of the
publicly shared templates. Read-only and free against any account.

`gpu_ids` feeds `gpu_type_id` in
[gpu-pod](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/gpu-pod)
and
[cluster](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/cluster);
`gpu_pools` maps a GPU type to the pool the
[serverless-endpoint](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/serverless-endpoint)
module takes; `cpu_types` gives the flavors and vCPU ranges for
[cpu-pod](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/cpu-pod)
and
[serverless-endpoint-cpu](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/serverless-endpoint-cpu);
`global_network_ids` lists where a pod can enable global networking;
`catalog_template_ids` are ready-made `template_id` values.

```bash
terraform apply -var catalog_slice=verified -var cpu_type_id=cpu5c
```

The account's own templates come from the
[template](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/template)
module's listing instead.

## Usage

```hcl
module "catalog_discovery" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/catalog-discovery?ref=v2.0.0"

  gpu_type_id    = "NVIDIA GeForce RTX 4090"
  data_center_id = "EU-RO-1"
  catalog_slice  = "official"
}
```

## Resources and data sources

- [`runpod_gpu_types`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/gpu_types)
- [`runpod_gpu_type`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/gpu_type)
- [`runpod_cpu_types`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/cpu_types)
- [`runpod_cpu_type`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/cpu_type)
- [`runpod_data_centers`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/data_centers)
- [`runpod_data_center`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/data_center)
- [`runpod_catalog_templates`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/catalog_templates)

## Configuration

```terraform
# The catalogs a configuration picks its identifiers from: GPU types and
# Serverless pools, CPU flavors, data centers, and the publicly shared
# templates. Every read is free and safe against any account.
data "runpod_gpu_types" "all" {}

data "runpod_gpu_type" "this" {
  id = var.gpu_type_id
}

data "runpod_cpu_types" "all" {}

# Flavor identifiers vary by account visibility, so the by-id CPU lookup runs
# only when one is given.
data "runpod_cpu_type" "this" {
  count = var.cpu_type_id == null ? 0 : 1

  id = var.cpu_type_id
}

data "runpod_data_centers" "all" {}

data "runpod_data_center" "this" {
  id = var.data_center_id
}

data "runpod_catalog_templates" "this" {
  source = var.catalog_slice
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0/v2/modules/catalog-discovery/README.md).
