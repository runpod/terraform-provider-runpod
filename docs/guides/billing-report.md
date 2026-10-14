---
page_title: "Billing report"
subcategory: "Discovery and reporting"
description: |-
  Read-only spend reporting: the account-wide total plus one data source per billed resource kind (pods, Serverless, endpoints, network volumes and clusters).
---

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
per-record GPU type and time fields live in the
[legacy](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/legacy)
module.

## Usage

```hcl
module "billing_report" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/billing-report?ref=v2.0.0"

  bucket_size = "day"
  last_n      = 7
}
```

## Resources and data sources

- [`runpod_billing`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/billing)
- [`runpod_pod_billing`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/pod_billing)
- [`runpod_serverless_billing`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/serverless_billing)
- [`runpod_endpoint_billing`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_billing)
- [`runpod_network_volume_billing`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/network_volume_billing)
- [`runpod_cluster_billing`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/cluster_billing)

## Configuration

```terraform
# Time-bucketed spend over the last last_n buckets: the account-wide total
# plus one data source per billed resource kind. Every read is free.
data "runpod_billing" "this" {
  bucket_size = var.bucket_size
  last_n      = var.last_n
}

data "runpod_pod_billing" "this" {
  bucket_size = var.bucket_size
  last_n      = var.last_n
  pod_id      = var.pod_id
}

data "runpod_serverless_billing" "this" {
  bucket_size   = var.bucket_size
  last_n        = var.last_n
  serverless_id = var.serverless_id
}

data "runpod_endpoint_billing" "this" {
  bucket_size = var.bucket_size
  last_n      = var.last_n
}

data "runpod_network_volume_billing" "this" {
  bucket_size       = var.bucket_size
  last_n            = var.last_n
  network_volume_id = var.network_volume_id
}

data "runpod_cluster_billing" "this" {
  bucket_size = var.bucket_size
  last_n      = var.last_n
  cluster_id  = var.cluster_id
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0/v2/modules/billing-report/README.md).
