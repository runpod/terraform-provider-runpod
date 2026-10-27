# Catalog discovery

Reads the catalogs every other module picks its identifiers from: the GPU
types with their Serverless pools and prices, the CPU flavors with their
vCPU bounds, the data centers with their capabilities, and one slice of the
publicly shared templates. Read-only and free against any account.

`gpu_ids` feeds `gpu_type_id` in [gpu-pod](../gpu-pod) and
[cluster](../cluster); `gpu_pools` maps a GPU type to the pool the
[serverless-endpoint](../serverless-endpoint) module takes; `cpu_types` gives
the flavors and vCPU ranges for [cpu-pod](../cpu-pod) and
[serverless-endpoint-cpu](../serverless-endpoint-cpu); `global_network_ids`
lists where a pod can enable global networking; `catalog_template_ids` are
ready-made `template_id` values.

```bash
terraform apply -var catalog_slice=verified -var cpu_type_id=cpu5c
```

The account's own templates come from the [template](../template) module's
listing instead.

Build and run it with the shared flow in [../README.md](../README.md).

## Usage

```hcl
module "catalog_discovery" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/catalog-discovery?ref=v2.0.0"

  gpu_type_id    = "NVIDIA GeForce RTX 4090"
  data_center_id = "EU-RO-1"
  catalog_slice  = "official"
}
```

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| terraform | >= 1.6 |
| runpod | ~> 2.0 |

## Providers

| Name | Version |
| ---- | ------- |
| runpod | ~> 2.0 |

## Resources

| Name | Type |
| ---- | ---- |
| [runpod_catalog_templates.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/catalog_templates) | data source |
| [runpod_cpu_type.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/cpu_type) | data source |
| [runpod_cpu_types.all](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/cpu_types) | data source |
| [runpod_data_center.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/data_center) | data source |
| [runpod_data_centers.all](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/data_centers) | data source |
| [runpod_gpu_type.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/gpu_type) | data source |
| [runpod_gpu_types.all](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/gpu_types) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| catalog\_slice | Public template catalog slice to read: official (Runpod-curated), verified (Runpod-verified community templates), or community (everything else shared publicly). | `string` | `"official"` | no |
| cpu\_type\_id | CPU flavor to look up in detail. Null skips the lookup, since flavor identifiers vary by account visibility. | `string` | `null` | no |
| data\_center\_id | Data center to look up in detail. | `string` | `"EU-RO-1"` | no |
| gpu\_type\_id | GPU type to look up in detail, e.g. for its Serverless pool and prices. | `string` | `"NVIDIA GeForce RTX 4090"` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| catalog\_template\_ids | Catalog template identifiers, usable as template\_id in the pod and endpoint modules. |
| catalog\_templates | Public templates in the selected catalog slice. |
| cpu\_type | The looked-up CPU flavor; null when cpu\_type\_id is unset. |
| cpu\_types | Full CPU flavor catalog with vCPU bounds and pricing, for the cpu-pod and serverless-endpoint-cpu modules. |
| data\_center | The looked-up data center: name, region and capabilities. |
| data\_centers | All data centers with region, compliance, and capabilities. |
| global\_network\_ids | Data centers where global networking is available. |
| gpu\_ids | GPU type identifiers, usable as gpu\_type\_id in the gpu-pod and cluster modules. |
| gpu\_pools | Serverless GPU pool per GPU type, keyed by type id, for the serverless-endpoint module's gpu\_pools; types outside any pool are omitted. |
| gpu\_type | The looked-up GPU type: name, pool, memory, prices and cloud availability. |
| gpu\_types | Full GPU catalog with per-cloud availability and pricing. |
| serverless\_template\_images | Images of the catalog templates that configure Serverless workers. |
<!-- END_TF_DOCS -->
