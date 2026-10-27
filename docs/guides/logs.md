---
page_title: "Logs"
subcategory: "Use-case modules"
description: |-
  Bounded reads of the two live log streams: a pod's container or system output, and a Serverless worker's.
---

# Logs

Bounded reads of the two live log streams: a pod's container or system
output, and a Serverless worker's. Read-only and free. Both reads are
optional: give `pod_id`, or `endpoint_id` with `worker_id`, or both.

```bash
terraform apply -var pod_id=gfj8b292vyg08g -var tail=50
terraform apply -var endpoint_id=ep_abc123 -var worker_id=worker_abc
```

The routes are event streams, not files. A read collects the `tail`
backfill and stops as soon as it has that many lines or the stream goes
quiet, and never runs longer than `read_timeout_seconds` (10 by default, 60
at most). `log_source` picks the container stream or the host lifecycle one.
It carries the data sources' `source` argument under another name because
`source` is reserved inside module blocks. Worker ids change as an endpoint
scales; take a current one from the
[serverless-endpoint](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/serverless-endpoint)
module's `worker_ids`.

## Usage

```hcl
module "logs" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/logs?ref=v2.0.0"

  pod_id     = "gfj8b292vyg08g"
  log_source = "container"
  tail       = 50
}
```

## Resources and data sources

- [`runpod_pod_logs`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/pod_logs)
- [`runpod_endpoint_worker_logs`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_worker_logs)

## Configuration

```terraform
# Bounded reads of the two live log streams. Each is a real event stream:
# the read returns the `tail` backfill and stops, or stops after
# read_timeout_seconds with whatever arrived. Both are optional so one module
# call can read either or both.
data "runpod_pod_logs" "this" {
  count = var.pod_id == null ? 0 : 1

  pod_id               = var.pod_id
  source               = var.log_source
  tail                 = var.tail
  read_timeout_seconds = var.read_timeout_seconds
}

data "runpod_endpoint_worker_logs" "this" {
  count = var.worker_id == null ? 0 : 1

  endpoint_id          = var.endpoint_id
  worker_id            = var.worker_id
  source               = var.log_source
  tail                 = var.tail
  read_timeout_seconds = var.read_timeout_seconds

  lifecycle {
    precondition {
      condition     = var.endpoint_id != null
      error_message = "worker_id needs the endpoint_id the worker belongs to."
    }
  }
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0/v2/modules/logs/README.md).
