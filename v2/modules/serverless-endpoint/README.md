# Serverless endpoint (GPU)

Creates a GPU Serverless endpoint from an image or a template, scaled to zero
workers by default so it costs nothing until requests arrive, and reads back
its operational state: current workers, container builds, one build in
detail, and the configuration releases with the current rollout. For CPU
workers use [serverless-endpoint-cpu](../serverless-endpoint-cpu).

`image` or `template_id`: an image builds the endpoint directly; a template
(see [template](../template)) supplies the container settings instead. Set
`ports` together with `endpoint_type = "LOAD_BALANCER"` and
`scaling_type = "REQUEST_COUNT"` to route requests straight to the worker's
own HTTP paths instead of through the managed queue; `request_urls` then
carries the base and health URLs rather than the job operations.

On the GPU side, `gpu_pools` selects the pool (the `pool` field in the GPU
catalog, see [catalog-discovery](../catalog-discovery)), `excluded_types`
subtracts specific cards, `allowed_cuda_versions` / `min_cuda_version`
constrain worker placement, and `flashboot` selects the cold-start
acceleration mode. A private worker image needs a `registry` credential (see
[registry](../registry)); `network_volumes` mounts volumes from
[network-volume](../network-volume) into every worker.

The build reads only return anything for an endpoint built from a Git
repository; `build_id` is optional and skips the by-id read when null.
Worker ids change as the endpoint scales; `worker_ids` is the input the
[logs](../logs) module takes for a worker's log stream.

The deprecated inputs and outputs (`gpu_type_priority`, `template_version`,
`users`, `version`, `workers`) go through the REST v1 compatibility shim and
exist for 1.x configurations; `compute_type`, also deprecated, is derived from
what REST v2 reports.

Build and run it with the shared flow in [../README.md](../README.md), then
`terraform destroy`.

## Usage

```hcl
module "serverless_endpoint" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/serverless-endpoint?ref=v2.0.0"

  name        = "inference"
  gpu_pools   = ["ADA_24"]
  workers_max = 3
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
| [runpod_serverless_endpoint.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/serverless_endpoint) | resource |
| [runpod_endpoint_build.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_build) | data source |
| [runpod_endpoint_builds.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_builds) | data source |
| [runpod_endpoint_releases.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_releases) | data source |
| [runpod_endpoint_workers.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/endpoint_workers) | data source |
| [runpod_serverless_endpoints.all](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/serverless_endpoints) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| allowed\_cuda\_versions | Exact set of acceptable CUDA versions for worker placement, e.g. ["12.8"]. Null accepts any; a non-empty set cannot be combined with min\_cuda\_version. | `list(string)` | `null` | no |
| build\_id | Identifier of one of the endpoint's container builds to read in detail. Null skips that read; only an endpoint built from a Git repository has builds. | `string` | `null` | no |
| compute\_type | Deprecated. Worker compute family, preserving the 1.x compute\_type attribute. Null derives it from gpu\_pools; a value other than GPU is rejected here. | `string` | `null` | no |
| data\_center\_ids | Data centers workers may be placed in. Null lets the scheduler choose; a network volume requires the workers to land in the volume's data center. | `list(string)` | `null` | no |
| disk | Worker container disk in GB. Null uses the API default. | `number` | `null` | no |
| endpoint\_type | Request routing: QUEUE or LOAD\_BALANCER. | `string` | `"QUEUE"` | no |
| env | Environment variables set on every worker. | `map(string)` | `{}` | no |
| excluded\_types | GPU type IDs to subtract from the pools, e.g. ["NVIDIA L40"]. Null uses the whole pool. | `list(string)` | `null` | no |
| flashboot | FlashBoot cold-start acceleration: OFF, FLASHBOOT, or PRIORITY\_FLASHBOOT. Null uses the API default. | `string` | `null` | no |
| gpu\_count | GPUs per worker. | `number` | `1` | no |
| gpu\_pools | Serverless GPU pool IDs (the pool field in the GPU catalog, see the catalog-discovery module). | `list(string)` | <pre>[<br/>  "ADA_24"<br/>]</pre> | no |
| gpu\_type\_priority | Deprecated. How the scheduler orders the GPU types inside the selected pools, e.g. "availability", preserving the 1.x gpu\_type\_priority attribute. Nothing reads it back. | `string` | `null` | no |
| idle\_timeout | Seconds an idle worker stays up before scaling down, 1 to 3600. Null uses the API default. | `number` | `null` | no |
| image | Docker image reference for workers. Null when template\_id supplies one. | `string` | `"runpod/mock-worker:dev"` | no |
| min\_cuda\_version | Lowest acceptable CUDA version for worker placement, e.g. "12.1". Null sets no floor. | `string` | `null` | no |
| name | Endpoint name. | `string` | `"example-endpoint"` | no |
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
| build\_image | Image the build named by build\_id produced; null when build\_id is unset. |
| build\_statuses | Build state per container build, newest first as the API returns them. Empty for an endpoint deployed from a prebuilt image. |
| compute\_type | Deprecated. Worker compute family; GPU for this module. |
| cpu | CPU configurations of the endpoint; always null for a GPU endpoint. |
| created\_at | When the endpoint was created. |
| endpoint\_ids | Identifiers of every Serverless endpoint on the account, including this one. |
| flashboot | Effective FlashBoot mode. |
| gpu\_pools | GPU pools workers are placed on. |
| id | Endpoint identifier. |
| image | Image the workers run, whether set directly or resolved from a template. |
| min\_cuda\_version | Effective CUDA floor for worker placement. |
| name | Endpoint name. |
| release\_versions | Configuration version per release. |
| request\_urls | Invocation URLs for the endpoint: the job operations for a QUEUE endpoint, or base and health for a LOAD\_BALANCER one. |
| rollout | How far the current configuration version has reached the endpoint's workers. |
| stale\_worker\_ids | Workers still running an older configuration version. |
| template\_version | Deprecated. Version of the template the endpoint was built from; null when it was built from an image. |
| timeout | Effective per-request timeout in milliseconds. |
| users | Deprecated. Account that owns the endpoint, as a one-element list. |
| version | Deprecated. The endpoint's configuration version. |
| worker\_ids | Identifiers of the endpoint's current workers; the input for the logs module. |
| worker\_summary | Worker counts by status, from runpod\_endpoint\_workers. |
| workers | Deprecated. Workers allocated at the moment the endpoint was read; worker\_ids is the supported view of the same thing. |
<!-- END_TF_DOCS -->
