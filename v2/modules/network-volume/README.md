# Network volume

Creates a persistent network storage volume and lists every volume on the
account. Billed for the allocated storage while it exists.

The volume is bound to its data center: a pod from [gpu-pod](../gpu-pod) or
[cpu-pod](../cpu-pod) mounts it through `network_volumes` and has to be
placed in the same data center (`data_center_ids`); the Serverless endpoint
modules take the `id` in their `network_volumes` list. Size can grow in
place but never shrink.

Build and run it with the shared flow in [../README.md](../README.md), then
`terraform destroy`.

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
| [runpod_network_volume.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/network_volume) | resource |
| [runpod_network_volumes.all](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/network_volumes) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| data\_center | Data center to create the volume in (see the catalog-discovery module). Immutable; pods and workers that mount the volume have to be placed there. | `string` | `"EU-RO-1"` | no |
| name | Volume name. | `string` | `"example-volume"` | no |
| size | Storage to allocate in GB, 10 to 4096. Can grow, never shrink. | `number` | `10` | no |
| type | Storage tier: STANDARD or HIGH\_PERFORMANCE. Null uses the data center default. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| data\_center | Data center the volume lives in; pass it as data\_center\_ids to whatever mounts it. |
| id | Volume identifier, the volume\_id the pod modules and the network\_volumes list of the endpoint modules take. |
| name | Volume name. |
| network\_volume\_ids | Identifiers of every network volume on the account, including this one. |
| size | Allocated storage in GB. |
| type | Effective storage tier (API default when not configured). |
<!-- END_TF_DOCS -->
