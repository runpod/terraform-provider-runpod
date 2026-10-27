# Legacy shims (deprecated)

The deprecated 1.x surfaces the 2.0 provider still serves: the host machine
resource and its two data sources and the account identity read over the
GraphQL API, because REST v2 has no route for them, and the three 1.x billing
data sources (`runpod_billing_pod`, `runpod_billing_network_volume`,
`runpod_billing_endpoint`) in their 1.x record shape over the REST v2 billing
routes. The endpoint records' `time_billed_ms` comes from the GraphQL API's
serverless billing, and a `gpu_type_id` read of `runpod_billing_pod` from the
GraphQL GPU pod billing, per GPU type as REST v1 reported it. Otherwise the pod
and network volume records' `time_billed_ms` and `disk_space_billed_gb`, and
the pod records' `gpu_type_id` and `endpoint_id`, read null, because no API
reports them per pod or per volume. **Every part of
this module is deprecated**, warns on each plan, and is removed once REST v2
reports the same thing or the 1.x shape is dropped. New configurations use the
[billing-report](../billing-report) module and never register machines from
Terraform.

The [migration guide](../../../docs/guides/migrating-from-1-x.md) lists every
shim with the 1.x attribute or data source it preserves and what replaces it.

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

Build and run it with the shared flow in [../README.md](../README.md), then
`terraform destroy` if a machine was registered.

## Usage

```hcl
module "legacy" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/legacy?ref=v2.0.0"

  bucket_size = "day"
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
| end\_time | End of the reported 1.x billing window: an RFC 3339 timestamp, or a date or a datetime without a zone, read as UTC; null ends it at the current time. The window is the 30 days before it. | `string` | `null` | no |
| endpoint\_id | Restrict 1.x Serverless endpoint billing to one endpoint; null reports every endpoint. | `string` | `null` | no |
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
| network\_volume\_record\_count | Deprecated. Number of 1.x network volume billing records in the window. |
| pod\_record\_count | Deprecated. Number of 1.x pod billing records in the window. |
| pod\_spend | Deprecated. Pod spend over the window, in USD. |
<!-- END_TF_DOCS -->
