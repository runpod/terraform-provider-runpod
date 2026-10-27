# Global volume

Creates a global volume and reads it back by id. A global volume is
usage-billed storage with no data center: a `runpod_pod` mounts it through
its `global_volumes` attribute wherever the pod is placed, and a GPU
`runpod_serverless_endpoint` mounts it on every worker through its own
`global_volumes`. Only the name can change.

Build and run it with the shared flow in [../README.md](../README.md), then
`terraform destroy`.

## Usage

```hcl
module "global_volume" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/global-volume?ref=v2.0.0"

  name = "datasets"
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
| [runpod_global_volume.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/global_volume) | resource |
| [runpod_global_volume.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/global_volume) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| name | Volume name, 1 to 191 characters with no leading or trailing whitespace. Renamed in place. | `string` | `"example-global-volume"` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| created\_at | When the volume was created, in RFC 3339 format. |
| id | Volume identifier, the volume\_id a pod's global\_volumes and the id an endpoint's global\_volumes take. |
| lookup\_name | The name the runpod\_global\_volume data source reads back by id. |
| name | Volume name. |
<!-- END_TF_DOCS -->
