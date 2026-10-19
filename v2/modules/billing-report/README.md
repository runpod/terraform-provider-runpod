# Billing report

Read-only spend reporting: the account-wide total plus one data source per
billed resource kind (pods, Serverless, Runpod's public endpoints, network
volumes and clusters). Every read is time-bucketed; `last_n` with
`bucket_size` asks for the last N buckets ending with the current one.

```bash
terraform apply -var bucket_size=month -var last_n=3
```

Add a filter to narrow a route to one resource:

```bash
terraform apply -var pod_id=pod_abc123 -var cluster_id=cluster_abc123
```

Without a filter the API emits one record per resource per bucket, so the
per-bucket map can be long; the `*_spend` outputs are the summed window.
These are the REST v2 billing routes; the 1.x billing data sources with their
per-record GPU type and time fields live in the [legacy](../legacy) module.

Build and run it with the shared flow in [../README.md](../README.md).

## Usage

```hcl
module "billing_report" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/billing-report?ref=v2.0.0-rc.3"

  bucket_size = "day"
  last_n      = 7
}
```

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| terraform | >= 1.6 |
| runpod | 2.0.0-rc.3 |

## Providers

| Name | Version |
| ---- | ------- |
| runpod | 2.0.0-rc.3 |

## Resources

| Name | Type |
| ---- | ---- |
| [runpod_billing.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/data-sources/billing) | data source |
| [runpod_cluster_billing.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/data-sources/cluster_billing) | data source |
| [runpod_endpoint_billing.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/data-sources/endpoint_billing) | data source |
| [runpod_network_volume_billing.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/data-sources/network_volume_billing) | data source |
| [runpod_pod_billing.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/data-sources/pod_billing) | data source |
| [runpod_serverless_billing.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/data-sources/serverless_billing) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| bucket\_size | Length of each time bucket: hour, day, week, month, or year. | `string` | `"day"` | no |
| cluster\_id | Restrict cluster billing to one cluster; null reports every cluster. | `string` | `null` | no |
| last\_n | Number of buckets to return, ending with the current one. | `number` | `7` | no |
| network\_volume\_id | Restrict network volume billing to one volume; null reports every volume. | `string` | `null` | no |
| pod\_id | Restrict pod billing to one pod; null reports every pod. | `string` | `null` | no |
| serverless\_id | Restrict serverless billing to one endpoint; null reports every endpoint. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| cluster\_spend | Instant Cluster spend over the window, in USD. |
| endpoint\_spend | Spend on Runpod's public endpoints over the window, in USD. |
| network\_volume\_spend | Network volume storage spend over the window, in USD. |
| pod\_count | Number of distinct pods the records span. |
| pod\_spend | Pod spend over the window, in USD. |
| serverless\_spend | Serverless spend over the window, in USD. |
| spend\_by\_bucket | Total spend per time bucket, keyed by the bucket's start. |
| total\_spend | Total spend over the window, in USD. |
| window | The window the API resolved the request to. |
<!-- END_TF_DOCS -->
