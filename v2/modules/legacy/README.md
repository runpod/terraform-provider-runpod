# Legacy shims (deprecated)

Everything the 2.0 provider still serves from the GraphQL and REST v1 APIs
because REST v2 has no route for it: the host machine resource and its two
data sources, the account identity read, and the three 1.x billing data
sources (`runpod_billing_pod`, `runpod_billing_network_volume`,
`runpod_billing_endpoint`) with their per-record `gpu_type_id`,
`disk_space_billed_gb`, `endpoint_id` and `time_billed_ms` fields. **Every
part of this module is deprecated**, warns on each plan, and is removed once
REST v2 reports the same thing. New configurations use the
[billing-report](../billing-report) module and never register machines from
Terraform.

The [migration guide](../../docs/guides/migrating-from-1-x.md) lists every
shim with the 1.x attribute or data source it preserves and what replaces it.

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

Build and run it with the shared flow in [../README.md](../README.md), then
`terraform destroy` if a machine was registered.

## Usage

```hcl
module "legacy" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/legacy?ref=v2.0.0"

  bucket_size = "day"
  gpu_type_id = "NVIDIA A100 80GB PCIe"
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
| [runpod_machine.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/machine) | resource |
| [runpod_billing_endpoint.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/billing_endpoint) | data source |
| [runpod_billing_network_volume.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/billing_network_volume) | data source |
| [runpod_billing_pod.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/billing_pod) | data source |
| [runpod_machine.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/machine) | data source |
| [runpod_machines.all](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/machines) | data source |
| [runpod_user.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/user) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| bucket\_size | Length of each 1.x billing bucket: hour, day, week, month, or year. | `string` | `"day"` | no |
| create\_machine | Register a host machine through the deprecated GraphQL shim. Needs a host account; a plain account cannot add machines. | `bool` | `false` | no |
| end\_time | End of the reported 1.x billing window as an ISO datetime; null ends it at the current time. There is no start time: the route decides how far back the buckets go. | `string` | `null` | no |
| endpoint\_id | Restrict 1.x Serverless endpoint billing to one endpoint; null reports every endpoint. | `string` | `null` | no |
| gpu\_type\_id | Restrict 1.x pod billing to pods running one GPU type, the one filter REST v2 billing cannot do. Null reports every GPU type. | `string` | `null` | no |
| list\_machines | List the account's host machines through the deprecated GraphQL shim. Needs a host account; creating a machine lists them too. | `bool` | `false` | no |
| machine\_name | Machine name, the only attribute the deprecated machineAdd mutation accepts. | `string` | `"example-machine"` | no |
| network\_volume\_id | Restrict 1.x network volume billing to one volume; null reports every volume. | `string` | `null` | no |
| pod\_id | Restrict 1.x pod billing to one pod; null reports every pod. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| account\_email | Deprecated. Email of the account the API key belongs to. |
| account\_id | Deprecated. Identifier of the account the API key belongs to. |
| endpoint\_record\_count | Deprecated. Number of 1.x Serverless endpoint billing records in the window. |
| endpoint\_spend | Deprecated. Serverless endpoint spend over the window, in USD. |
| machine\_gpu\_total | Deprecated. GPUs the API reports for the registered machine, read back through the data source; null unless create\_machine is set. |
| machine\_id | Deprecated. Identifier of the registered machine; null unless create\_machine is set. |
| machine\_name | Deprecated. Name of the registered machine; null unless create\_machine is set. |
| machine\_names | Deprecated. Name per machine on the account, keyed by machine id. Null unless list\_machines or create\_machine is set. |
| network\_volume\_disk\_gb | Deprecated. Disk space billed across the network volume records, in GB. REST v2 billing does not report it. |
| network\_volume\_record\_count | Deprecated. Number of 1.x network volume billing records in the window. |
| pod\_gpu\_types | Deprecated. GPU types the pod records bill, deduplicated. REST v2 billing does not report them. |
| pod\_record\_count | Deprecated. Number of 1.x pod billing records in the window. |
| pod\_spend | Deprecated. Pod spend over the window, in USD. |
| pod\_time\_billed\_ms | Deprecated. Pod time billed over the window, in milliseconds. REST v2 billing does not report it. |
<!-- END_TF_DOCS -->
