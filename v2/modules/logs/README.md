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
[serverless-endpoint](../serverless-endpoint) module's `worker_ids`.

Build and run it with the shared flow in [../README.md](../README.md).

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
| [runpod_endpoint_worker_logs.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_worker_logs) | data source |
| [runpod_pod_logs.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/pod_logs) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| endpoint\_id | Serverless endpoint the worker belongs to. Required with worker\_id. | `string` | `null` | no |
| log\_source | Log source to read: "container" for the container's stdout and stderr, "system" for the host lifecycle stream, or null for both. | `string` | `"container"` | no |
| pod\_id | Pod whose log stream to read. Null skips the pod read. | `string` | `null` | no |
| read\_timeout\_seconds | Hard bound on each read, for a stream that keeps producing. Null uses the provider default of 10; 60 is the ceiling, since the provider's HTTP client closes the connection there. | `number` | `null` | no |
| tail | Historical lines to backfill from the stream, 0 to 5000. The read returns as soon as that many events arrived or the stream went quiet. | `number` | `20` | no |
| worker\_id | Worker whose log stream to read, from the serverless-endpoint module's worker\_ids. Null skips the worker read. Workers come and go with load, so the id has to be current. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| pod\_log\_last\_id | Cursor of the last pod log event read, for a later read's last\_event\_id; null when pod\_id is unset. |
| pod\_log\_lines | Log lines the pod's stream delivered; null when pod\_id is unset. |
| worker\_log\_last\_id | Cursor of the last worker log event read, for a later read's last\_event\_id; null when worker\_id is unset. |
| worker\_log\_lines | Log lines the worker's stream delivered; null when worker\_id is unset. |
<!-- END_TF_DOCS -->
