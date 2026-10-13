# Serverless endpoint (CPU)

Creates a CPU Serverless endpoint: `cpu` lists the flavor / vCPU pairs
workers may be placed on instead of GPU pools. Scaled to zero workers by
default, so it costs nothing until requests arrive. Flavor identifiers and
their valid vCPU counts come from the CPU catalog (see
[catalog-discovery](../catalog-discovery)).

Everything else works as in the GPU [serverless-endpoint](../serverless-endpoint)
module: `image` or `template_id`, queue or load-balancer routing, autoscaling
bounds, a `registry` credential for a private image, and `network_volumes`
for storage shared by every worker.

The deprecated inputs and outputs (`cpu_flavor_priority`, `users`, `version`,
`workers`) go through the REST v1 compatibility shim and exist for 1.x
configurations; `compute_type`, also deprecated, is derived from what REST v2
reports.

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
| [runpod_serverless_endpoint.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/serverless_endpoint) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| compute\_type | Deprecated. Worker compute family, preserving the 1.x compute\_type attribute. Null derives it from cpu; a value other than CPU is rejected here. | `string` | `null` | no |
| cpu | CPU configurations workers may be placed on: a flavor from the CPU catalog (see the catalog-discovery module) and a vCPU count valid for it. | <pre>list(object({<br/>    cpu_type_id = string<br/>    vcpu_count  = number<br/>  }))</pre> | <pre>[<br/>  {<br/>    "cpu_type_id": "cpu5c",<br/>    "vcpu_count": 4<br/>  }<br/>]</pre> | no |
| cpu\_flavor\_priority | Deprecated. How the scheduler orders the configured CPU flavors, e.g. "availability", preserving the 1.x cpu\_flavor\_priority attribute. Nothing reads it back. | `string` | `null` | no |
| data\_center\_ids | Data centers workers may be placed in. Null lets the scheduler choose; a network volume requires the workers to land in the volume's data center. | `list(string)` | `null` | no |
| disk | Worker container disk in GB. Null uses the API default. | `number` | `null` | no |
| endpoint\_type | Request routing: QUEUE or LOAD\_BALANCER. | `string` | `"QUEUE"` | no |
| env | Environment variables set on every worker. | `map(string)` | `{}` | no |
| idle\_timeout | Seconds an idle worker stays up before scaling down, 1 to 3600. Null uses the API default. | `number` | `null` | no |
| image | Docker image reference for workers. Null when template\_id supplies one. | `string` | `"runpod/mock-worker:dev"` | no |
| name | Endpoint name. | `string` | `"example-cpu-endpoint"` | no |
| network\_volumes | Network volume IDs (see the network-volume module) mounted into every worker. Null attaches none. | `list(string)` | `null` | no |
| ports | Worker HTTP ports a LOAD\_BALANCER endpoint routes to, e.g. ["8000/http"]. Null for a QUEUE endpoint, which takes requests through the managed queue. | `list(string)` | `null` | no |
| registry | Registry credential ID (see the registry module) for pulling a private worker image. Null for public images. | `string` | `null` | no |
| scaling\_type | Autoscaling signal: QUEUE\_DELAY or REQUEST\_COUNT. | `string` | `"QUEUE_DELAY"` | no |
| scaling\_value | Scaler threshold: seconds of queue delay, or in-flight request count. | `number` | `4` | no |
| template\_id | Serverless template to base the endpoint on (see the template module). Null builds it from image alone. | `string` | `null` | no |
| timeout | Per-request execution timeout in milliseconds. Null uses the API default. | `number` | `null` | no |
| workers\_max | Maximum number of workers. | `number` | `1` | no |
| workers\_min | Minimum number of workers. Zero costs nothing until requests arrive. | `number` | `0` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| compute\_type | Deprecated. Worker compute family; CPU for this module. |
| cpu | CPU configurations workers are placed on. |
| created\_at | When the endpoint was created. |
| gpu\_pools | GPU pools of the endpoint; always null for a CPU endpoint. |
| id | Endpoint identifier. |
| image | Image the workers run, whether set directly or resolved from a template. |
| name | Endpoint name. |
| request\_urls | Invocation URLs for the endpoint: the job operations for a QUEUE endpoint, or base and health for a LOAD\_BALANCER one. |
| timeout | Effective per-request timeout in milliseconds. |
| users | Deprecated. Account that owns the endpoint, as a one-element list. |
| version | Deprecated. The endpoint's configuration version. |
| workers | Deprecated. Workers allocated at the moment the endpoint was read. |
<!-- END_TF_DOCS -->
