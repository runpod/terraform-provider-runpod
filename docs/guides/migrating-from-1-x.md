---
page_title: "Migrating from 1.x to 2.0.0"
subcategory: "Upgrade"
description: |-
  Attribute-by-attribute mapping from the 1.0.8 provider to 2.0.0, with the configuration edit and state action every resource needs.
---

# Migrating from the Runpod Terraform provider 1.x to 2.0.0

## What changed and why

The 2.0.0 provider is generated from the Runpod REST v2 API and speaks only that API. The 1.x provider was
assembled from three different backends at once: GraphQL for machines and some pod paths, REST v1 for the
user and billing reads, and REST v2 for the rest. That is why 1.x attribute names came in three naming
styles and why several 1.x attributes were accepted by Terraform but silently ignored by the service.
2.0.0 has one source of truth, so every attribute maps to a documented v2 field, names follow the v2
schema (`image`, not `image_name`; `disk`, not `container_disk_in_gb`), and anything the API never
supported is absent rather than accepted-and-dropped.

"What 1.x did" means what **1.0.8** did. 1.0.9 shipped but could not be used: it cannot serve its own
schema and cannot configure nine of its ten resources, so 1.0.8 is the last release that worked and the
baseline for everything 2.0.0 preserves; see "Why 1.0.9 is not the baseline" at the end.

What 1.x did and v2 has no field for yet is kept rather than dropped, on its old backend and
marked deprecated: the machine surface and the user identity read on GraphQL; the three 1.x billing
data sources and a handful of attributes on pods, templates and Serverless endpoints on REST v1. Terraform
warns about each one on every plan, and each goes away when v2 reports the same thing; see "Deprecated
shims" at the end. The mapping tables below name them row by row.

This is a breaking release. Every resource needs a configuration edit, and every resource needs a state
action, described in the state migration section at the end.

## Provider block and authentication

1.x:

```hcl
terraform {
  required_providers {
    runpod = {
      source = "runpod/runpod"
    }
  }
}

provider "runpod" {
  api_key = var.runpod_api_key
}
```

2.0.0:

```hcl
terraform {
  required_version = ">= 1.6"

  required_providers {
    runpod = {
      source  = "runpod/runpod"
      version = "~> 2.0"
    }
  }
}

provider "runpod" {}
```

Authentication is unchanged in substance: `api_key`, falling back to the `RUNPOD_API_KEY` environment
variable. The provider errors at configure time if neither is set.

The `base_url` argument now points at a different host. In 1.0.8 the `base_url` argument and the
`RUNPOD_BASE_URL` environment variable both set the REST v1 host, `https://rest.runpod.io/v1`. In 2.0.0
`base_url` sets the REST v2 host that everything except the deprecated shims calls, falling back to
`RUNPOD_API_BASE_URL` and then the production API. A 1.0.8 configuration that sets `base_url` must move
that value to `rest_v1_url` (below): left in `base_url`, it sends every v2 call to the REST v1 host. Set
`base_url` or `RUNPOD_API_BASE_URL` only to point the provider at a non-production v2 host.

Two more provider arguments exist only for the deprecated shims, and everything on REST v2 ignores them:

- `graphql_url` — the GraphQL endpoint used by the `runpod_machine` resource, the `runpod_machine` /
  `runpod_machines` data sources and the `runpod_user` data source (REST v1 `/user` is gone, so the
  account read moved to the GraphQL `myself` query; only `id` and `email` have a source there). Falls back
  to `RUNPOD_GRAPHQL_URL`, then the production GraphQL host.
- `rest_v1_url` — the REST v1 base URL used by the three 1.x billing data
  sources, and the deprecated attributes on `runpod_pod`, `runpod_template` (resource and data source)
  and `runpod_serverless_endpoint`. Falls back to `RUNPOD_REST_V1_URL`, then `RUNPOD_BASE_URL` (the 1.x
  name for this host), then the production REST v1 host.

Set them only if you use the deprecated surfaces against a non-production environment. The
`RUNPOD_BASE_URL` variable is unchanged for a 1.0.8 setup: it set the REST v1 host then and sets the REST
v1 shim host now, so a 1.0.8 staging setup keeps pointing its shim reads at staging. The v2 client does
not read it; point that at a non-production host with `RUNPOD_API_BASE_URL` or `base_url`.

## Upgrade steps

1. Pin the new major version and raise the Terraform floor:

   ```hcl
   terraform {
     required_version = ">= 1.6"

     required_providers {
       runpod = {
         source  = "runpod/runpod"
         version = "~> 2.0"
       }
     }
   }
   ```

   Run `terraform init -upgrade`. A release candidate, such as `2.0.0-rc.2`, installs only through an exact
   pin, `version = "2.0.0-rc.2"`; `~> 2.0` does not select it.

2. Before editing anything, record the identifiers you will need to re-import. `terraform state list`
   plus `terraform show -json` against the old state, or the `id` outputs you already have.
   Do this while the 1.x provider is still the one in the lock file.

3. Rewrite the configuration resource by resource using the mapping tables below. Rename the three
   resource types that changed name (`runpod_endpoint`, `runpod_container_registry_auth` and
   `runpod_ecr_delegation`), rename the two data sources that changed with them
   (`runpod_container_registry_auths`, `runpod_ecr_delegations`), and delete the resources that no longer
   exist.

4. Run the state actions from the state migration section. 2.0.0 ships no state upgraders (every schema
   is at version 0 in both majors), so a 1.x state entry whose attribute set changed cannot be decoded by
   2.0.0. The fix is `terraform state rm` followed by `terraform import`. Those two commands change state
   only, but for a pod the first apply after them can replace it: read "Before importing a pod you want to
   keep" in the state migration section first.

5. `terraform plan`. What you should see:

   - Templates, network volumes, endpoints, machines and account keys: no changes, if the rewritten
     configuration matches what the API reports. A mismatch on an immutable attribute plans a
     **replacement**. Read every planned replacement before applying.
   - Imported pods: a **replacement** for every create-time attribute the v2 read does not return and the
     configuration sets, unless it is covered as described in the state migration section. A pod
     replacement destroys the pod and creates a new one. Do not apply a plan that replaces a pod you meant
     to keep.
   - Registry credentials and ECR delegations: a **create**, because 2.0.0 cannot import them (see below).
   - Log, worker and billing data sources: always shown as needing a refresh, because they read live data.

6. Expect genuine replacements where 1.x let you change something the API cannot change in place:
   `gpu_type_id`, `gpu_count`, `cloud`, `cpu_type_id`, `vcpu_count`, `template_id`, the CUDA constraints
   and the host-size floors all force a new pod in 2.0.0, and switching a Serverless endpoint between the
   GPU and CPU families replaces the endpoint.

---

## Resources

### runpod_pod (resource)

2.0.0 equivalent: `runpod_pod`. Same resource type name, same id format (the pod id). The attribute set
changed substantially, and the two mount kinds moved into blocks.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `name` | `name` | Same, but now **required** |
| `image_name` | `image` | Renamed |
| `docker_args` | `args` | Renamed |
| `docker_entrypoint` | `docker_entrypoint` | Now read-only: 1.0.8 accepted a value here but never sent it. Remove it from the configuration and put the command in `args`. Read from the v2 pod's `entrypoint`, which the API decodes from `args` |
| `docker_start_cmd` | `docker_start_cmd` | Now read-only: 1.0.8 accepted a value here but never sent it. Remove it from the configuration and fold the command into `args`. Read from the v2 pod's `cmd`, which the API decodes from `args` |
| `container_disk_in_gb` | `disk` | Renamed |
| `cloud_type` | `cloud` | Renamed; values are `SECURE` and `COMMUNITY`, the 1.x `ALL` value is gone. Changing it replaces the pod |
| `gpu_type_id` | `gpu_type_id` | Same. Changing it replaces the pod |
| `gpu_count` | `gpu_count` | Same. Changing it replaces the pod |
| `env` | `env` | Shape change: 1.x took a list of `"KEY=VALUE"` strings, 2.0.0 takes a map of string |
| `port` | — | Removed; it was schema-only in 1.x — declared, never sent, never read back. Use `ports` |
| `ports` | `ports` | Shape change: 1.x took one string, 2.0.0 takes a list, e.g. `["8888/http", "22/tcp"]` |
| `template_id` | `template_id` | Same. Must not be a serverless template; changing it replaces the pod |
| `start_ssh` | `start_ssh` | Same. Create-time only; changing it replaces the pod |
| `start_jupyter` | `start_jupyter` | Same. Create-time only; changing it replaces the pod |
| `volume_in_gb` | `persistent_volume.size` | Moved into block |
| `volume_mount_path` | `persistent_volume.path` | Moved into block |
| `volume_key` | — | Removed; schema-only in 1.x and no volume encryption key in the v2 API |
| `volume_encrypted` | `volume_encrypted` | Deprecated, read-only: read back from the REST v1 shim. It reports what the platform did; there is no way to ask for encryption |
| `network_volume_id` | `network_volumes[].volume_id` (with `path`) | Moved into block; the mount path is now explicit |
| `network_volume_ids` | `network_volumes` | Moved into block; a list of `{ volume_id, path }` objects |
| `machine_id` | `machine_id` | Deprecated, read-only: the machine the pod landed on, read back from the REST v1 shim. The v2 pod create has no machine pinning, so it cannot be set — constrain placement with `data_center_ids` and the host-size floors instead |
| `type` | — | Removed; spot pods were retired by Runpod in September 2026 (see "Removed and not coming back") |
| `interruptible` | — | Removed; spot pods were retired by Runpod |
| `bid_per_gpu` | — | Removed; spot pods were retired by Runpod |
| `stop_after` | — | Removed; schema-only in 1.x — the value was never sent — and no scheduled stop in the v2 API. Use `runpod_pod.desired_state` or `runpod_pod_action` |
| `terminate_after` | — | Removed; schema-only in 1.x and no scheduled terminate in the v2 API |
| `vcpu_count` | `vcpu_count` | Repurposed: computed in 1.x, an input for CPU pods in 2.0.0. Read the allocated count from the `runpod_pod` data source (`gpu_vcpu_count` / `cpu_vcpu_count`) |
| `memory_in_gb` | `memory_in_gb` | Same name, now from the v2 read; the `runpod_pod` data source splits it into `gpu_memory` / `cpu_memory` |
| `cost_per_hr` | `cost` | Renamed; USD per hour, from the v2 read |
| `locked` | `locked` | Same name, and now it reports: 1.x declared it and never filled it, 2.0.0 reads it from v2 |
| `cluster_ip` | `cluster_ip` | Same name, and now it reports: 1.x declared it and never filled it, 2.0.0 reads it from v2. The `runpod_cluster_pods` data source reports it for every member |
| `id` | `id` | Same |
| `status` | `status` | Same |
| `created_at` | `created_at` | Same |
| — | `cpu_type_id` | New: a CPU-only pod. Exactly one of `gpu_type_id` or `cpu_type_id` must be set |
| — | `desired_state` | New: `RUNNING` or `EXITED`, reconciled on every apply |
| — | `data_center_ids` | New: placement preference; the scheduler's pick lands in `data_center_id` |
| — | `data_center_id` | New, read-only: where the pod was placed |
| — | `registry` | New: registry credential id for a private image |
| — | `global_networking` | New: private cross-datacenter IP |
| — | `min_cuda_version`, `allowed_cuda_versions` | New: CUDA floor, or an exact set. Mutually exclusive |
| — | `min_ram_per_gpu`, `min_vcpu_count_per_gpu` | New: host-size placement filters, per requested GPU |

Before (1.x):

```hcl
resource "runpod_pod" "training" {
  name                 = "training-pod"
  image_name           = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  gpu_type_id          = "NVIDIA GeForce RTX 4090"
  gpu_count            = 1
  cloud_type           = "SECURE"
  container_disk_in_gb = 20
  start_ssh            = true
  start_jupyter        = true
  ports                = "8888/http,22/tcp"
  env                  = ["MODEL_NAME=llama-3"]
  volume_in_gb         = 50
  volume_mount_path    = "/workspace"
}
```

After (2.0.0):

```hcl
resource "runpod_pod" "training" {
  name          = "training-pod"
  image         = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  gpu_type_id   = "NVIDIA GeForce RTX 4090"
  gpu_count     = 1
  cloud         = "SECURE"
  disk          = 20
  start_ssh     = true
  start_jupyter = true
  ports         = ["8888/http", "22/tcp"]

  env = {
    MODEL_NAME = "llama-3"
  }

  persistent_volume = {
    size = 50
    path = "/workspace"
  }
}
```

A network volume instead of host-local storage. The two are mutually exclusive, and the volume must be in
the same data center as the pod:

```hcl
resource "runpod_pod" "training" {
  name        = "training-pod"
  image       = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  gpu_type_id = "NVIDIA GeForce RTX 4090"

  network_volumes = [{
    volume_id = runpod_network_volume.data.id
    path      = "/runpod-volume"
  }]
}
```

### runpod_endpoint (resource)

2.0.0 equivalent: **`runpod_serverless_endpoint`** — the resource type was renamed. The largest semantic
change is GPU selection: 1.x named individual GPU types, 2.0.0 names Serverless GPU *pools* and subtracts
exceptions from them.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `name` | `name` | Same |
| `image_name` | `image` | Renamed |
| `container_disk_in_gb` | `disk` | Renamed |
| `env` | `env` | Same (map of string in both) |
| `template_id` | `template_id` | Same. Must be a serverless template |
| `template_version` | `template_version` | Deprecated, read-only: read back from the REST v1 shim. Releases are also reported by the `runpod_endpoint_releases` data source |
| `gpu_type_id` | `gpu_pools` | Replaced: a list of Serverless pool ids, e.g. `["ADA_24"]`. Read the pool from the `pool` field of `runpod_gpu_types` |
| `gpu_type_ids` | `gpu_pools` | Replaced, same as above |
| `gpu_type_priority` | `gpu_type_priority` | Deprecated: the v2 endpoint has no priority ordering inside a pool, so a configured value is applied through the REST v1 shim and, as in 1.x, never read back |
| `gpu_count` | `gpu_count` | Same |
| `compute_type` | `compute_type` | Deprecated: the family follows from whether `gpu_pools` or `cpu` is set, so the value is derived from what v2 reports. Keeping it in the configuration is allowed as long as it agrees with the block; a value that contradicts it is rejected |
| `cpu_flavor_ids` | `cpu[].cpu_type_id` | Moved into block |
| `vcpu_count` | `cpu[].vcpu_count` | Moved into block; one entry per eligible configuration |
| `cpu_flavor_priority` | `cpu_flavor_priority` | Deprecated: applied through the REST v1 shim when set and never read back, as in 1.x |
| `cloud_type` | — | Removed; it exists only in 1.0.9, which never ran, and the v2 Serverless create takes no cloud tier |
| `scaler_type` | `scaling_type` | Renamed; values `QUEUE_DELAY` or `REQUEST_COUNT` |
| `scaler_value` | `scaling_value` | Renamed; now **required** alongside `scaling_type` |
| `workers_min` | `workers_min` | Same |
| `workers_max` | `workers_max` | Same |
| `idle_timeout` | `idle_timeout` | Same; 1 to 3600 seconds |
| `execution_timeout_ms` | `timeout` | Renamed; still milliseconds |
| `flashboot` | `flashboot` | Type change: boolean in 1.x, a string enum in 2.0.0 — `OFF`, `FLASHBOOT`, or `PRIORITY_FLASHBOOT` |
| `allowed_cuda_versions` | `allowed_cuda_versions` | Same |
| `min_cuda_version` | `min_cuda_version` | Same; mutually exclusive with a non-empty `allowed_cuda_versions` |
| `data_center_ids` | `data_center_ids` | Same |
| `network_volume_id` | `network_volumes` | Replaced by a list of volume ids |
| `network_volume_ids` | `network_volumes` | Renamed |
| `users` | `users` | Deprecated, read-only: the owning account as a one-element list, read back from the REST v1 shim. It was never an access list |
| `version` | `version` | Deprecated, read-only: read back from the REST v1 shim. The `runpod_endpoint_releases` data source reports `endpoint_version` |
| `workers` | `workers` | Deprecated, read-only: the same one-per-worker snapshot 1.x reported, with `id`, `pod_id`, `status`, `uptime_ms`, `start_time` and `last_busy_ms`, read back from the REST v1 shim. `runpod_endpoint_workers` is the supported view |
| `id` | `id` | Same |
| `created_at` | `created_at` | Same |
| — | `endpoint_type` | New and **required**: `QUEUE` or `LOAD_BALANCER`. Changing it replaces the endpoint |
| — | `ports` | New: the worker HTTP ports a `LOAD_BALANCER` endpoint routes to |
| — | `excluded_types` | New: GPU type ids subtracted from the selected pools |
| — | `registry` | New: registry credential id for a private worker image |

Before (1.x):

```hcl
resource "runpod_endpoint" "inference" {
  name                 = "inference-endpoint"
  image_name           = "runpod/mock-worker:dev"
  gpu_type_ids         = ["NVIDIA RTX 4000 Ada Generation"]
  gpu_count            = 1
  flashboot            = true
  scaler_type          = "QUEUE_DELAY"
  scaler_value         = 4
  workers_min          = 0
  workers_max          = 3
  idle_timeout         = 5
  execution_timeout_ms = 600000
}
```

After (2.0.0):

```hcl
resource "runpod_serverless_endpoint" "inference" {
  name          = "inference-endpoint"
  image         = "runpod/mock-worker:dev"
  endpoint_type = "QUEUE"
  gpu_pools     = ["ADA_24"]
  gpu_count     = 1
  flashboot     = "FLASHBOOT"
  scaling_type  = "QUEUE_DELAY"
  scaling_value = 4
  workers_min   = 0
  workers_max   = 3
  idle_timeout  = 5
  timeout       = 600000

  excluded_types   = ["NVIDIA L40"]
  min_cuda_version = "12.1"
}
```

### runpod_network_volume (resource)

2.0.0 equivalent: `runpod_network_volume`. Same resource type name, same id format. Two renames.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `name` | `name` | Same |
| `size` | `size` | Same; 10 to 4096 GB, can grow, never shrink |
| `data_center_id` | `data_center` | Renamed; immutable, changing it replaces the volume |
| `storage_tier` | `type` | Renamed; `STANDARD` or `HIGH_PERFORMANCE`, immutable |
| `id` | `id` | Same |

Before (1.x):

```hcl
resource "runpod_network_volume" "datasets" {
  name           = "datasets"
  size           = 100
  data_center_id = "EU-RO-1"
  storage_tier   = "HIGH_PERFORMANCE"
}
```

After (2.0.0):

```hcl
resource "runpod_network_volume" "datasets" {
  name        = "datasets"
  size        = 100
  data_center = "EU-RO-1"
  type        = "HIGH_PERFORMANCE"
}
```

### runpod_template (resource)

2.0.0 equivalent: `runpod_template`. Same resource type name, same id format. The container settings come
from the v2 template object and the single persistent mount moved into the `persistent_volume` block; the
marketplace and accounting fields and `docker_entrypoint` are kept as deprecated REST v1 shims, so a 1.x
configuration keeps planning unchanged.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `name` | `name` | Same |
| `image_name` | `image` | Renamed; now **required** |
| `container_disk_in_gb` | `disk` | Renamed |
| `category` | `category` | Same; `CPU`, `NVIDIA`, or `AMD` |
| `env` | `env` | Same |
| `ports` | `ports` | Same |
| `is_public` | `public` | Renamed |
| `is_serverless` | `serverless` | Renamed |
| `docker_start_cmd` | `args` | Renamed and reshaped: a list in 1.x, a single string in 2.0.0 |
| `docker_entrypoint` | `docker_entrypoint` | Deprecated: applied through the REST v1 shim on create and update, and read back from it |
| `container_registry_auth_id` | `registry` | Renamed |
| `volume_in_gb` | `persistent_volume.size` | Moved into block |
| `volume_mount_path` | `persistent_volume.path` | Moved into block |
| `readme` | `readme` | Deprecated: applied through the REST v1 shim on create and update, and read back from it; set it to `""` to clear the text |
| `earned` | `earned` | Deprecated, read-only: read back from the REST v1 shim |
| `is_runpod` | `is_runpod` | Deprecated, read-only: read back from the REST v1 shim |
| `runtime_in_min` | `runtime_in_min` | Deprecated, read-only: read back from the REST v1 shim |
| `id` | `id` | Same |

Before (1.x):

```hcl
resource "runpod_template" "pytorch" {
  name                 = "pytorch-notebook"
  image_name           = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  container_disk_in_gb = 20
  is_public            = false
  is_serverless        = false
  ports                = ["8888/http", "22/tcp"]
  docker_start_cmd     = ["bash", "-lc", "start.sh"]

  env = {
    MODEL_NAME = "llama-3"
  }
}
```

After (2.0.0):

```hcl
resource "runpod_template" "pytorch" {
  name       = "pytorch-notebook"
  image      = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  disk       = 20
  public     = false
  serverless = false
  ports      = ["8888/http", "22/tcp"]
  args       = "bash -lc start.sh"

  env = {
    MODEL_NAME = "llama-3"
  }
}
```

### runpod_container_registry_auth (resource)

2.0.0 equivalent: **`runpod_registry`** — the resource type was renamed. Every attribute kept its name.
The API has no update operation, so any change replaces the credential, and 2.0.0 does not support
importing one.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `name` | `name` | Same |
| `username` | `username` | Same; not returned by the API, tracked from configuration |
| `password` | `password` | Same; sensitive, not returned by the API, tracked from configuration |
| `id` | `id` | Same |

Before (1.x):

```hcl
resource "runpod_container_registry_auth" "private" {
  name     = "private-images"
  username = "runpod-puller"
  password = var.registry_password
}

resource "runpod_pod" "private" {
  name       = "private-image-pod"
  image_name = "ghcr.io/example/trainer:1.0.0"
}
```

After (2.0.0), with the pod referencing the credential explicitly:

```hcl
resource "runpod_registry" "private" {
  name     = "private-images"
  username = "runpod-puller"
  password = var.registry_password
}

resource "runpod_pod" "private" {
  name        = "private-image-pod"
  image       = "ghcr.io/example/trainer:1.0.0"
  gpu_type_id = "NVIDIA GeForce RTX 4090"
  registry    = runpod_registry.private.id
}
```

### runpod_ecr_delegation (resource)

2.0.0 equivalent: **`runpod_registry_delegation`** — the resource type was renamed. Every attribute kept
its name, and the read-only set is identical. The API has no update operation, so any change replaces the
delegation, and 2.0.0 does not support importing one.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `resource` | `resource` | Same; now **required**. Not echoed by the API, tracked from configuration |
| `name` | `name` | Same, optional |
| `id` | `id` | Same |
| `aws_user` | `aws_user` | Same |
| `aws_region` | `aws_region` | Same |
| `repository` | `repository` | Same |
| `tag` | `tag` | Same |
| `docker_registry_uri` | `docker_registry_uri` | Same |
| `created_at` | `created_at` | Same |

Before (1.x):

```hcl
resource "runpod_ecr_delegation" "ecr" {
  name     = "ecr-trainers"
  resource = "arn:aws:ecr:us-east-1:123456789012:repository/trainers"
}
```

After (2.0.0):

```hcl
resource "runpod_registry_delegation" "ecr" {
  name     = "ecr-trainers"
  resource = "arn:aws:ecr:us-east-1:123456789012:repository/trainers"
}
```

### runpod_pod_action (resource)

2.0.0 equivalent: `runpod_pod_action`. Same resource type name, but the semantics are now explicit: the
resource is imperative. It reconciles nothing, refreshes to nothing, and destroying it only forgets it;
the pod is left as it is. The action runs on create and again whenever `action` or `triggers` change, and
the only pod state it keeps is the `status` the action itself answered with.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `pod_id` | `pod_id` | Same; pointing it at a different pod replaces the resource and runs the action on the new pod |
| `action` | `action` | Same names, `start` / `stop` / `restart` / `terminate`. 1.x also documented `resume` and `reset`; use `start` and `restart` |
| `status` | `status` | Kept, read-only, and not a shim: the v2 action response carries it, so 1.x's GraphQL mutation field has a v2 equivalent. It is the status at the moment the action was accepted, as 1.x reported it, not a live value — a refresh does not update it, and `terminate` answers with no body, so it leaves this null. Read live state from the `runpod_pod` data source |
| — | `triggers` | New: a map of arbitrary values that re-runs the action when any of them changes |
| — | `id` | New, read-only: the pod this resource acts on — one action runner per pod |

Do not combine this with `runpod_pod.desired_state` on the same pod. Both manage the same transitions, and
each apply would undo the other. Pick `desired_state` for a run state Terraform should keep, and this
resource for a one-off restart or a stop tied to something else changing.

Before (1.x):

```hcl
resource "runpod_pod_action" "stop" {
  pod_id = runpod_pod.batch.id
  action = "stop"
}
```

After (2.0.0), a restart that re-runs when the image tag moves:

```hcl
resource "runpod_pod_action" "restart_on_image_change" {
  pod_id = runpod_pod.jupyter.id
  action = "restart"

  triggers = {
    image = var.image
  }
}
```

Or, if what you actually wanted was a run state Terraform keeps:

```hcl
resource "runpod_pod" "batch" {
  name          = "batch-pod"
  image         = var.image
  gpu_type_id   = "NVIDIA GeForce RTX 4090"
  desired_state = "EXITED"
}
```

### runpod_machine (resource)

2.0.0 equivalent: `runpod_machine`, kept as a deprecated shim on the GraphQL API. Same resource type name,
same id format, and every 1.x attribute kept its name: `name`, `gpu_type_id`, `gpu_count`, `cpu_count`,
`memory_in_gb`, `disk_in_gb`, `data_center_id`, `location`, `secure_cloud`, `listed`, `bid_price_per_gpu`,
`host_price_per_gpu`, `id`, `maintenance_mode`, `verified`, `runpod_ip`. No configuration change is needed
beyond the state action. Only `name` and `bid_price_per_gpu` are settable — the create mutation takes no
other input, and `bid_price_per_gpu` appears in no GraphQL document, so it reports back what the
configuration set, exactly as in 1.x. See "Deprecated shims" below.

Two 1.x mutations never matched the GraphQL schema and are corrected here: a rename now sends
`machineId` (1.x sent `id`, which `MachineEditNameInput` does not declare), and a destroy sends
`machineDelete(input: { machineId })`, which returns nothing (1.x passed a bare `machineId` argument and
selected fields on the result). A machine the API no longer reports, which it answers with an empty
object rather than an error, is now removed from state instead of being kept.

### runpod_endpoint_job (resource)

**Removed.** It exists only in 1.0.9, which never ran at all, and it called Serverless job routes under
the v2 host that were cancelled before they shipped, so nothing ever worked through it. Jobs are runtime
objects served by the Serverless runtime API (a different host
and auth surface), not infrastructure Terraform should own. Submit jobs from your application or from a
`null_resource`-style caller against the runtime API; there is no provider replacement and none is planned.

### runpod_endpoint_worker (resource)

**Removed.** It exists only in 1.0.9, which never ran, and it called worker update and delete routes that
do not exist in v2; only the worker list and worker logs do. Use the `runpod_endpoint_workers` and
`runpod_endpoint_worker_logs` data sources
to observe workers, and the endpoint's `workers_min` / `workers_max` / `scaling_type` / `scaling_value` to
control them. Individual workers are not addressable.

---

## Data sources

### runpod_pod (data source)

2.0.0 equivalent: `runpod_pod`. Same name, same required `id`. It now reports far more than 1.x did:
live cost, runtime utilization, SSH details, mounts, cluster membership.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `id` | `id` | Same |
| `name` | `name` | Same |
| `image_name` | `image` | Renamed |
| `docker_args` | `args` | Renamed |
| `container_disk_in_gb` | `disk` | Renamed |
| `cost_per_hr` | `cost` | Renamed; USD per hour |
| `env` | `env` | Shape change: list of `"KEY=VALUE"` in 1.x, map of string in 2.0.0 |
| `ports` | `ports` | Shape change: one string in 1.x, a list in 2.0.0 |
| `gpu_count` | `gpu_count` | Same |
| `gpu_type_id` | `gpu_type_id` | Same |
| `memory_in_gb` | `gpu_memory` / `cpu_memory` | Renamed and split by pod family |
| `status` | `status` | Same |
| `desired_status` | — | Removed; no 1.x read of this data source ever completed (see below), and only the reported `status` remains |
| `template_id` | `template` | Renamed |
| `volume_in_gb` | `mounts.persistent.size` | Moved into block |
| `volume_mount_path` | `mounts.persistent.path` | Moved into block |
| `machine_id` | — | Removed; the 1.x data source queried it but could never write it to state (see below). The `runpod_pod` *resource* reports a deprecated `machine_id` read-back |
| `machine_type` | — | Removed; same, and it has no REST v1 counterpart either, so even a shim would need GraphQL |
| `created_at` | `created_at` | Same |
| — | `cloud`, `data_center_id`, `started_at`, `locked`, `registry`, `actions` | New |
| — | `cpu_type_id`, `cpu_vcpu_count`, `gpu_vcpu_count` | New |
| — | `cuda_version` | New; as reported by the host machine |
| — | `global_networking`, `global_networking_ip`, `global_networking_internal_dns` | New |
| — | `mounts.network` | New; the pod's network volume mounts |
| — | `runtime` | New; CPU, memory and per-GPU utilization, uptime, live port mappings |
| — | `ssh.direct`, `ssh.proxy` | New; ready-to-run SSH commands |
| — | `cluster` | New; cluster id, rank, role and overlay IP for a member pod |

Why three rows say the 1.x data source could never write them: it built its state model with an untyped
empty list for `env` while its schema declared a list of string, and the state write is the last statement
of its only success branch. Every read failed on that conversion whatever the API returned, so nothing in
a 1.x configuration can have consumed `desired_status`, `machine_id` or `machine_type` from here. 2.0.0
fixes the data source rather than shimming those three.

### runpod_machine (data source)

2.0.0 equivalent: `runpod_machine`, a deprecated GraphQL shim. Same name, same required `id`, and every
attribute kept its name: `cpu_count`, `cpu_type_id`, `data_center_id`, `disk_reserved`, `disk_total`,
`gpu_reserved`, `gpu_total`, `gpu_type_id`, `host_price_per_gpu`, `listed`, `location`,
`maintenance_mode`, `memory_reserved`, `memory_total`, `name`, `runpod_ip`, `secure_cloud`, `verified`.

One behaviour note carried over from 1.x: `data_center_id` is always null here, because the GraphQL query
this data source ports does not select it. Use `runpod_machines` when you need the data center. An id
the API does not know, or a machine the key does not own, is now an error rather than a read whose
attributes are all null.

### runpod_machines (data source)

2.0.0 equivalent: `runpod_machines`, a deprecated GraphQL shim. Same name, same shape, and every nested
attribute kept its name: `data_center_id`, `gpu_total`, `gpu_type_id`, `id`, `listed`, `location`, `name`,
`secure_cloud`. It reads the account's machines through `myself { machines }`. 1.x queried the root
`machines` field, which lists the public supply and only answers support and host callers.

### runpod_gpu_types (data source)

2.0.0 equivalent: `runpod_gpu_types`, now read from the v2 catalog API. The list attribute is still
`gpu_types`; the nested fields were renamed to the catalog's names.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `gpu_types[].id` | `gpu_types[].id` | Same |
| `gpu_types[].display_name` | `gpu_types[].name` | Renamed |
| `gpu_types[].memory_in_gb` | `gpu_types[].memory` | Renamed; VRAM in GB |
| `gpu_types[].secure_cloud` | `gpu_types[].secure` | Renamed |
| `gpu_types[].secure_price` | `gpu_types[].price_secure` | Renamed; USD per hour |
| `gpu_types[].community_price` | `gpu_types[].price_community` | Renamed; USD per hour |
| `gpu_types[].cuda_cores` | — | Removed; not in the v2 catalog |
| `gpu_types[].manufacturer` | — | Removed; not in the v2 catalog |
| — | `gpu_types[].community` | New: availability on community cloud |
| — | `gpu_types[].pool` | New: the Serverless pool id for `runpod_serverless_endpoint.gpu_pools` |

A new single-item companion exists: `runpod_gpu_type`, looked up by `id`, with the same fields.

### runpod_data_centers (data source)

2.0.0 equivalent: `runpod_data_centers`, now read from the v2 catalog API. The list attribute is still
`data_centers`.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `data_centers[].id` | `data_centers[].id` | Same |
| `data_centers[].name` | `data_centers[].name` | Same |
| `data_centers[].global_network` | `data_centers[].global_network` | Same |
| `data_centers[].location` | `data_centers[].region` | Renamed |
| — | `data_centers[].compliance` | New: compliance certifications held |
| — | `data_centers[].network_volume_types` | New: storage tiers offered |

A new single-item companion exists: `runpod_data_center`, looked up by `id`, with the same fields.

### runpod_template (data source)

2.0.0 equivalent: `runpod_template`. Same name, same required `id`.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `id` | `id` | Same |
| `name` | `name` | Same |
| `image_name` | `image` | Renamed |
| `container_disk_in_gb` | `disk` | Renamed |
| `category` | `category` | Same |
| `env` | `env` | Same |
| `ports` | `ports` | Same |
| `is_public` | `public` | Renamed |
| `is_serverless` | `serverless` | Renamed |
| `container_registry_auth_id` | `registry` | Renamed |
| `docker_start_cmd` | `args` | Renamed and reshaped: list to single string |
| `docker_entrypoint` | `docker_entrypoint` | Same: read from the v2 template's `entrypoint`, which the API decodes from `args` |
| `volume_in_gb` | `mounts.persistent.size` | Moved into block |
| `volume_mount_path` | `mounts.persistent.path` | Moved into block |
| `readme` | `readme` | Deprecated, read-only: read back from the REST v1 shim |
| `earned` | `earned` | Deprecated, read-only: read back from the REST v1 shim |
| `is_runpod` | `is_runpod` | Deprecated, read-only: read back from the REST v1 shim |
| `runtime_in_min` | `runtime_in_min` | Deprecated, read-only: read back from the REST v1 shim |
| — | `allowed_cuda_versions` | New |
| — | `start_ssh`, `start_jupyter` | New |

### runpod_container_registry_auths (data source)

2.0.0 equivalent: **`runpod_registries`** — renamed, and narrowed to what the v2 API returns.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `container_registry_auths` | `registries` | Renamed list attribute |
| `container_registry_auths[].id` | `registries[].id` | Same |
| `container_registry_auths[].name` | `registries[].name` | Same |
| `container_registry_auths[].username` | `registries[].username` | Deprecated and always null: REST v2 treats the username as write-only, and the GraphQL credential type has no username field |
| `container_registry_auths[].password` | — | Removed; it exists only in 1.0.9, which never ran, and the v2 list does not return it |
| `container_registry_auths[].created_at` | — | Removed; 1.0.9 only, same reason |
| `container_registry_auths[].updated_at` | — | Removed; 1.0.9 only, same reason |

### runpod_ecr_delegations (data source)

2.0.0 equivalent: **`runpod_registry_delegations`** — the data source was renamed along with the
resource, and the list attribute with it.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `ecr_delegations` | `delegations` | Renamed list attribute |
| `ecr_delegations[].id` | `delegations[].id` | Same |
| `ecr_delegations[].name` | `delegations[].name` | Same; null when the delegation was created without one |
| `ecr_delegations[].aws_user` | `delegations[].aws_user` | Same |
| `ecr_delegations[].aws_region` | `delegations[].aws_region` | Same |
| `ecr_delegations[].repository` | `delegations[].repository` | Same |
| `ecr_delegations[].tag` | `delegations[].tag` | Same |
| `ecr_delegations[].docker_registry_uri` | `delegations[].docker_registry_uri` | Same |
| `ecr_delegations[].created_at` | `delegations[].created_at` | Same; RFC 3339 |
| `ecr_delegations[].resource` | — | Removed; the API does not echo the delegated ARN, which is also why the resource cannot be imported. Read it from your own configuration |
| — | `delegations[].delegator_user_id` | New: the Runpod user that created the delegation |

### runpod_endpoint_jobs (data source)

**Removed.** The v2 Serverless job subroutes this read were cancelled and never shipped; jobs live on the
Serverless runtime API. See `runpod_endpoint_job` above. No provider replacement.

### runpod_endpoint_job_logs (data source)

**Removed.** Same reason: job logs are a runtime-API surface. For worker-level output use
`runpod_endpoint_worker_logs`, and for pod output the new `runpod_pod_logs`.

### runpod_endpoint_workers (data source)

2.0.0 equivalent: `runpod_endpoint_workers`. Same name, same required `endpoint_id`, richer per-worker
data and a status summary.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `endpoint_id` | `endpoint_id` | Same |
| `id` | — | Removed; the data source no longer carries a synthetic id |
| `workers[].id` | `workers[].id` | Same |
| `workers[].status` | `workers[].status` | Same; `IDLE`, `INITIALIZING`, `RUNNING`, `THROTTLED`, `UNHEALTHY` |
| `workers[].start_time` | `workers[].started_at` | Renamed; RFC 3339 |
| `workers[].uptime_ms` | `workers[].uptime_seconds` | Renamed, and the unit changed from milliseconds to seconds |
| `workers[].pod_id` | — | Not in the v2 worker list. The deprecated `runpod_serverless_endpoint.workers` snapshot still carries `pod_id`, on REST v1 |
| `workers[].last_busy_ms` | — | Not in the v2 worker list; same deprecated fallback as `pod_id` |
| — | `workers[].data_center_id`, `workers[].gpu_type_id`, `workers[].gpu_count`, `workers[].image` | New |
| — | `workers[].is_stale` | New: whether the worker runs an older configuration version |
| — | `summary` | New: counts by status — `total`, `idle`, `initializing`, `running`, `throttled`, `unhealthy` |
| — | `endpoint_version` | New: the endpoint's current configuration version |

### runpod_endpoint_worker_logs (data source)

2.0.0 equivalent: `runpod_endpoint_worker_logs`. Same name, same required `endpoint_id` and `worker_id`.
The API serves logs as a live event stream, so a read collects what the stream delivers and ends when the
API closes it or the provider's 60-second HTTP timeout elapses.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `endpoint_id` | `endpoint_id` | Same |
| `worker_id` | `worker_id` | Same |
| `id` | — | Removed |
| `logs` | `lines` | Replaced: one string in 1.x, a list of line strings in 2.0.0. Use `join` to get the old shape |
| — | `entries` | New: per-event `line`, `timestamp` and `source` |
| — | `tail`, `since`, `last_event_id`, `source` | New: bound the read |

### runpod_billing_pod (data source)

2.0.0 equivalent: **`runpod_pod_billing`** — renamed, and re-pointed from REST v1 to the v2 billing
routes. Costs are now broken out by component instead of reported as billed time and billed disk.

`runpod_billing_pod` itself still exists, deprecated, with every 1.x argument and record field unchanged
on the REST v1 route. It stays only for the fields v2 billing does not carry yet (`gpu_type_id` as a
filter and as a record field, `time_billed_ms`, `disk_space_billed_gb`, `endpoint_id`). A configuration
that needs none of those should move to the table below. One behaviour change: a failed REST v1 read is
now an error rather than an empty result, because the records are this data source's whole body.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `bucket_size` | `bucket_size` | Same; `hour`, `day` (default), `week`, `month`, `year` |
| `end_time` | `end_time` | Same; RFC 3339, exclusive |
| `pod_id` | `pod_id` | Same filter |
| `gpu_type_id` | — | Not in the v2 route, which has no GPU type filter; keep `runpod_billing_pod` for it |
| `billing_records` | `records` | Renamed |
| `billing_records[].amount` | `records[].amounts.total_amount` | Moved into block; USD |
| `billing_records[].time` | `records[].start_time` (with `records[].end_time`) | Replaced: a bucket now reports both bounds |
| `billing_records[].pod_id` | `records[].pod_id` | Same |
| `billing_records[].time_billed_ms` | — | Not in the v2 route, which reports cost, not billed milliseconds; keep `runpod_billing_pod` for it |
| `billing_records[].disk_space_billed_gb` | — | Not in the v2 route: the disk *cost* is `records[].amounts.disk_amount` in USD; keep `runpod_billing_pod` for the billed GB |
| `billing_records[].endpoint_id` | — | Not in the v2 route; Serverless spend is `runpod_serverless_billing`, and `runpod_billing_pod` still reports the endpoint a worker pod served |
| — | `start_time`, `last_n` | New: `last_n` returns the last N buckets and is mutually exclusive with the explicit window |
| — | `records[].amounts.gpu_amount`, `.cpu_amount`, `.disk_amount` | New: cost components |
| — | `metadata.query`, `metadata.totals`, `metadata.record_count`, `metadata.unique_pod_count` | New: the resolved window and the totals over it |

### runpod_billing_network_volume (data source)

2.0.0 equivalent: **`runpod_network_volume_billing`** — renamed, re-pointed to the v2 billing routes, and
now split by storage tier.

`runpod_billing_network_volume` itself still exists, deprecated, unchanged on the REST v1 route, for the
same reason as the pod one: `time_billed_ms` and `disk_space_billed_gb` have no v2 equivalent. A failed
read is an error there.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `bucket_size` | `bucket_size` | Same |
| `end_time` | `end_time` | Same |
| `network_volume_id` | `network_volume_id` | Same filter |
| `billing_records` | `records` | Renamed |
| `billing_records[].amount` | `records[].amounts.total_amount` | Moved into block; USD |
| `billing_records[].network_volume_id` | `records[].network_volume_id` | Same |
| `billing_records[].time` | `records[].start_time` (with `records[].end_time`) | Replaced |
| `billing_records[].time_billed_ms` | — | Not in the v2 route; keep `runpod_billing_network_volume` for it |
| `billing_records[].disk_space_billed_gb` | — | Not in the v2 route, which reports storage cost in USD, not billed GB; keep `runpod_billing_network_volume` for it |
| — | `start_time`, `last_n` | New |
| — | `records[].amounts.standard_amount`, `.high_performance_amount` | New: per-tier cost |
| — | `metadata.*` | New, including `unique_network_volume_count` |

### runpod_billing_endpoint (data source)

2.0.0 equivalent: **`runpod_serverless_billing`**. Watch the name: 1.x `runpod_billing_endpoint` read the
REST v1 endpoints billing route filtered by `endpointId`, which meant Serverless endpoints. In 2.0.0 that
is `runpod_serverless_billing`, filtered by `serverless_id`. The separately named `runpod_endpoint_billing`
data source in 2.0.0 is a **different** thing: spend on Runpod's own public endpoints, with no resource
filter. Do not map to it by name.

`runpod_billing_endpoint` itself still exists, deprecated, unchanged on the REST v1 route, for
`time_billed_ms`. A failed read is an error there.

| 1.x attribute | 2.0.0 attribute | Note |
| --- | --- | --- |
| `bucket_size` | `bucket_size` | Same |
| `end_time` | `end_time` | Same |
| `endpoint_id` | `serverless_id` | Renamed |
| `billing_records` | `records` | Renamed |
| `billing_records[].amount` | `records[].amounts.total_amount` | Moved into block; USD |
| `billing_records[].endpoint_id` | `records[].serverless_id` | Renamed |
| `billing_records[].time` | `records[].start_time` (with `records[].end_time`) | Replaced |
| `billing_records[].time_billed_ms` | — | Not in the v2 route; keep `runpod_billing_endpoint` for it |
| — | `start_time`, `last_n` | New |
| — | `records[].amounts.gpu_amount`, `.cpu_amount`, `.disk_amount` | New: cost components; the platform fee is included in the GPU and CPU amounts |
| — | `metadata.*` | New, including `unique_serverless_count` |

### runpod_user (data source)

2.0.0 equivalent: `runpod_user`, kept as a deprecated shim that now reads the GraphQL `myself` query:
the REST v1 `/user` route 1.x read is gone from the v1 host (it answers 400, on 1.x too). Same name, no
arguments, and every attribute kept its name, but only `id` and `email` have a GraphQL source; `name`,
`pub_key`, `verified`, `cloud_type`, `gpu_limit`, `gpu_usage`, `storage_limit` and `storage_usage` read
null (the registered SSH keys are on the v2 SSH keys route). See "Deprecated shims" below.

---

## New in 2.0.0

- **Clusters** — multi-node clusters of identical pods on one private overlay network, with SLURM, Ray,
  training and application topologies.

  ```hcl
  resource "runpod_cluster" "training" {
    name = "llama-finetune"
    type = "SLURM"
    gpu_type_id = "NVIDIA H100 80GB HBM3"
    gpu_count_per_pod = 8
    pod_count = 4
    image = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  }
  ```

  Read them with `runpod_cluster`, `runpod_clusters` and `runpod_cluster_pods`.

- **Account SSH keys** — the account's registered public keys as a managed singleton. Every write replaces
  the whole document, so declare it once per account.

  ```hcl
  resource "runpod_account_ssh_keys" "this" {
    keys              = var.ssh_public_keys
    remove_on_destroy = false
  }
  ```

- **Catalog templates** — the publicly shared template catalog, one slice per read.

  ```hcl
  data "runpod_catalog_templates" "official" {
    source = "official"
  }
  ```

- **Serverless builds** — container builds of an endpoint built from a Git repository, with commit,
  status and failure detail.

  ```hcl
  data "runpod_endpoint_builds" "vllm" {
    endpoint_id = runpod_serverless_endpoint.inference.id
  }
  ```

  A single build is `runpod_endpoint_build`, keyed by `endpoint_id` and `id`.

- **Serverless releases** — configuration releases and the state of the current rollout, each release
  carrying a field-level diff against the one before it.

  ```hcl
  data "runpod_endpoint_releases" "vllm" {
    endpoint_id = runpod_serverless_endpoint.inference.id
  }
  ```

- **Billing data sources on the v2 routes** — six of them: `runpod_billing` (account-wide),
  `runpod_pod_billing`, `runpod_serverless_billing`, `runpod_endpoint_billing`,
  `runpod_network_volume_billing` and `runpod_cluster_billing`, all with the same window arguments and
  per-component totals.

  ```hcl
  data "runpod_billing" "last_week" {
    bucket_size = "day"
    last_n      = 7
  }
  ```

- **CPU pods and CPU Serverless endpoints** — a CPU flavor replaces the GPU type, with the vCPU count
  taken from the flavor's catalog range.

  ```hcl
  resource "runpod_pod" "preprocess" {
    name        = "preprocess-pod"
    image       = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
    cpu_type_id = "cpu5c"
    vcpu_count  = 4
  }
  ```

  On an endpoint, the `cpu` block lists every eligible configuration. The flavors themselves are in the
  new `runpod_cpu_type` and `runpod_cpu_types` catalog data sources.

- **CUDA and host-size constraints on placement** — an open-ended floor or an exact set, plus per-GPU RAM
  and vCPU floors on pods.

  ```hcl
  resource "runpod_pod" "strict" {
    name                   = "strict-pod"
    image                  = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
    gpu_type_id            = "NVIDIA GeForce RTX 4090"
    min_cuda_version       = "12.1"
    min_ram_per_gpu        = 8
    min_vcpu_count_per_gpu = 2
  }
  ```

- **FlashBoot as a tier** — cold-start acceleration is now a three-value setting rather than a boolean.

  ```hcl
  flashboot = "PRIORITY_FLASHBOOT"
  ```

- **Pod logs** — `runpod_pod_logs`, the pod-level counterpart to worker logs, bounded by `tail`, `since`
  or `last_event_id`.

- **Single-item catalog lookups** — `runpod_gpu_type`, `runpod_cpu_type` and `runpod_data_center`, so you
  no longer have to filter a whole list to read one entry.

- **Declarative pod run state** — `runpod_pod.desired_state` (`RUNNING` or `EXITED`) makes a pod stopped
  outside Terraform show up as a plan diff and get started again on the next apply.

---

## Removed and not coming back in 2.0.0

Everything in this section is something 1.x declared and never actually did. Anything 1.x did
is preserved; see "Deprecated shims" below. Each reason below rests on what 1.0.8, the last working 1.x
release, sent in its request bodies and read back from the responses.

- **Spot pods (`type = "SPOT"`, `interruptible`, `bid_per_gpu`).** Retired by the platform, not an API
  gap: the console stopped offering spot and interruptible pods in July 2026 and the APIs stopped
  accepting bids on 2026-09-11, so a pod create with a bid is now refused on GraphQL and REST alike.
  Before that date the 1.0.9 provider sent a bid field the REST v2 service never read, so a 1.0.9 "spot"
  pod was already scheduled and billed as on-demand. Migrate `interruptible = true` / `bid_per_gpu`
  configurations to on-demand pods.

- **`stop_after`, `terminate_after`, `port` and `volume_key` on `runpod_pod`.** All four were schema-only
  in every 1.x release: declared in the resource schema and referenced by no create, read, update or
  delete logic. The values stayed exactly as configured because they never
  appeared in a request body and were never read back, so no pod was ever scheduled to stop, terminated
  on a timer, given a single-port mapping or created with an encryption key. There is no scheduled stop
  or terminate and no volume encryption key in the v2 API either. Use `runpod_pod.desired_state` for a
  run state Terraform keeps, or `runpod_pod_action` with `stop` / `terminate` driven by a `triggers`
  value for a one-off; declare ports as a list in `ports`; read whether the platform encrypted the volume
  from the deprecated `volume_encrypted`. Two more 1.x schema-only attributes, `cluster_ip` and `locked`, keep their
  names in 2.0.0 and are now filled for real from the v2 read.

- **`desired_status`, `machine_id` and `machine_type` on the `runpod_pod` data source.** A provider-side
  defect made every 1.x read of that data source fail: it built its state model with an untyped empty
  list for `env` against a schema declaring a list of string, and the state write is the last statement
  of its only success branch. No read ever completed, whatever the API returned, so no configuration can
  have depended on these three. 2.0.0 fixes the data source; it reports `status`, and the `runpod_pod`
  resource reports `machine_id` as a deprecated read-back. `machine_type` has no REST v1 counterpart
  anywhere in the 1.x code, so it would have needed a new GraphQL shim to preserve something that never
  worked.

- **`cloud_type` on the endpoint, and `password` / `created_at` / `updated_at` on the registry list.**
  These four are absent from the 1.0.8 schema. They exist only in 1.0.9, which never ran — see "Why 1.0.9
  is not the baseline" below. The v2 Serverless create takes no cloud tier and the v2 registry list does
  not return the secret or the timestamps.

- **`runpod_endpoint_job` and `runpod_endpoint_worker` resources, `runpod_endpoint_jobs` and
  `runpod_endpoint_job_logs` data sources.** All four were added in 1.0.9, against Serverless job and
  worker subroutes under the v2 host that were cancelled in July 2026 before shipping — and 1.0.9 itself
  never ran, so no configuration ever used them. Runpod chose a different design on purpose:
  instead of proxying the job lifecycle through REST v2, the endpoint responses expose the runtime
  invocation URLs (run, runsync and the load-balancer base) and clients submit jobs straight to the
  Serverless runtime API, a different host and a different auth surface. Submitting work is not
  infrastructure state Terraform should reconcile, and workers are managed by the autoscaler rather than
  individually. Observe workers with `runpod_endpoint_workers` and `runpod_endpoint_worker_logs`, and
  control them through the endpoint's scaling attributes.

- **Machine pinning on pods.** `machine_id` was an input in the 1.x pod schema and the 1.0.8 create body
  never carried it, so pinning never worked there either; the v2 pod create has no machine parameter.
  Constrain placement with `data_center_ids` and the per-GPU RAM and vCPU floors. The machine the pod
  ended up on is still reported, as the deprecated `machine_id` read-back.

---

## Why 1.0.9 is not the baseline

2.0.0 preserves what **1.0.8** did, not what 1.0.9 declared. 1.0.9 as published could not be used:

- It refuses to serve its own schema. Two list attributes (`endpoint_jobs.jobs` and
  `endpoint_workers.workers`) declare none of required, optional or computed, which the plugin framework
  rejects, so no Terraform version can plan or apply with it.
- Nine of its ten resources panic during provider configuration on a type assertion the provider itself
  cannot satisfy: it hands each resource one client type and every resource asserts a different one.
- Its three billing data sources build their own client and resolve the host from the 1.x
  `RUNPOD_BASE_URL` variable, whose default moved to the v2 host in 1.0.9, so those reads went to a host
  that does not serve the REST v1 billing routes.

1.0.8, by contrast, builds, serves its schema, configures every resource and data source cleanly, and
completes an apply plus refresh end to end. That is why the shim list is drawn from what 1.0.8 sent and
read on the wire, and why an attribute that appears only in 1.0.9 is treated as something no
configuration can have depended on.

---

## Deprecated shims

Everything a 1.0.8 configuration used that REST v2 has no field for still works in 2.0.0, on the old
backend, marked deprecated in the schema. Terraform prints the deprecation warning on every plan, and the
attribute or data source goes away in a later release once REST v2 reports the same thing. These are the
only parts of the provider that do not speak v2, and the only ones that need the `graphql_url` /
`rest_v1_url` provider arguments. They call the old hosts through the Go SDK's deprecated `legacy`
package (`github.com/runpod/go-sdk/v2/legacy`), which ships alongside the v2 client and is removed with
the last of these shims.

How a failure on the old host behaves: a shim attribute nothing configured is a **read**: if the host does
not answer, the plan gets a warning and the attribute reads null, and the apply goes through. A shim
attribute the configuration *sets* is a **write**: if that fails, the apply fails, because the
configuration asked for something only that host can apply. The three deprecated billing data sources are
the exception: their records are their whole body, so any failed read is an error rather than a null list.

Whole surfaces:

- **Machines** — the `runpod_machine` resource and the `runpod_machine` / `runpod_machines` data sources
  call the GraphQL API, exactly as 1.x did: a create, a rename and a delete mutation, and a query per
  read. Every attribute name is unchanged. Only `name` and `bid_price_per_gpu` are settable — the create
  mutation takes no other input, `name` is also the only thing an update changes, and `bid_price_per_gpu`
  appears in no GraphQL document, so it reports back what the configuration set, as in 1.x. Everything
  else reports what the API assigned.
  **Removal condition:** they are removed when a `/v2/machines` surface exists and the resource is
  regenerated against it. If no such surface is planned, they are dropped rather than ported.

- **User** — the `runpod_user` data source reads the GraphQL `myself` query (REST v1 `/user` is gone).
  Every attribute name is unchanged; only `id` and `email` have a source, and the others read null.
  **Removal condition:** it is removed when a v2 account identity route exists (the v2 account billing
  read is spend, not identity).

- **The three 1.x billing data sources** — `runpod_billing_pod`, `runpod_billing_network_volume` and
  `runpod_billing_endpoint` keep their 1.x names, arguments and record fields and read the REST v1 billing
  routes. They stay only for the per-record fields v2 billing does not carry yet (`time_billed_ms`,
  `disk_space_billed_gb`, the pod record's `endpoint_id`, and the `gpu_type_id` filter). New configuration
  belongs on the v2 data sources — `runpod_pod_billing`, `runpod_network_volume_billing`,
  `runpod_serverless_billing` — which report cost components in USD.

Attributes on otherwise-v2 resources. Each is removed once REST v2 reports the same thing, and each
plan that touches one prints the warning:

- **`runpod_pod`** — `machine_id` and `volume_encrypted` are read back from a REST v1 pod read after the
  v2 read, on create, read and update. Both are read-only, because neither was ever a create input in
  1.0.8 either: the 1.0.8 create body never carried them. `docker_entrypoint` and `docker_start_cmd` are
  not shims: they are read from the v2 pod.

- **`runpod_serverless_endpoint`** — `gpu_type_priority` and `cpu_flavor_priority` are applied through a
  REST v1 PATCH on create and on every update that sets them, and are never read back, exactly as in 1.x,
  so the value stays as configured; `template_version`, `users`, `version` and `workers` (with `id`,
  `pod_id`, `status`, `uptime_ms`, `start_time` and `last_busy_ms`) are REST v1 read-backs, and when a
  priority is set the PATCH response supplies them, so the shim costs one request rather than two.
  `compute_type` is deprecated for a different reason: it restates what `gpu_pools` or `cpu` already says,
  so 2.0.0 derives it from the v2 response, rejects a value that contradicts the block, and writes nothing
  to the old host for it. It goes away once 1.x configurations have moved to `gpu_pools` and `cpu`, not on
  an API change.

- **`runpod_template` (resource)** — `docker_entrypoint` and `readme` are applied through a REST v1 PATCH
  on create and update when the configuration sets them, and read back from it, so the template's own
  value fills them in when the configuration leaves them out; `earned`, `is_runpod` and `runtime_in_min`
  are REST v1 read-backs.

- **`runpod_template` (data source)** — four of the same fields, all read-only REST v1 read-backs:
  `readme`, `earned`, `is_runpod`, `runtime_in_min`. The 1.x data source read them over GraphQL; REST v1
  carries all four, so the shim uses one host instead of two. `docker_entrypoint` is not a shim here: it is
  read from the v2 template.

- **`runpod_registries`** — `registries[].username` is kept so a 1.x configuration that references it
  still plans, and it always reads null. No supported API reports a credential's username: REST v2 treats
  it as write-only, and the GraphQL `ContainerRegistryAuth` type has no username field (there is no
  `containerRegistryAuths` root field either), so the provider sends no request for it.

Not a shim, despite looking like one: `runpod_pod_action.status`. 1.x read it off a GraphQL mutation
result, and the v2 action response carries the same thing, so it survives on REST v2 with no deprecated
host behind it.

Treat all of the shims as read-mostly and do not build new configuration on them.

---

## State migration

2.0.0 ships **no state upgraders** (every resource schema is at version 0 in both majors), so Terraform
will not rewrite a 1.x state entry into the 2.0.0 shape. Wherever the attribute set changed, the old state
entry cannot be decoded and must be removed and re-imported. `terraform state rm` and `terraform import`
change state only: running them stops no pod and deletes no volume. For a pod, the first `terraform apply`
after them can, as the next paragraph explains.

Do the state work after rewriting the configuration and before the first `terraform apply`.

**Before importing a pod you want to keep.** The v2 pod read does not return eight attributes a 2.0.0 pod
configuration can set: `template_id`, `start_ssh`, `start_jupyter`, `allowed_cuda_versions`,
`min_cuda_version`, `min_ram_per_gpu`, `min_vcpu_count_per_gpu` and `data_center_ids`. They are create-time
inputs, so `terraform import` leaves them empty in state, and every one of them the configuration sets is a
diff on the first plan. All eight force replacement: applying that plan deletes the pod permanently, with
everything not on a network volume, and creates a new pod with a new id. The "After (2.0.0)" pod example
above sets `start_ssh` and `start_jupyter`, so a pod migrated with it as written is replaced. There are two
safe ways through:

- **Leave a running 1.x pod on 1.x.** Do not remove and import it. Keep it in a root module that stays
  pinned to `version = "<= 1.0.8"` (not `~> 1.0`, which selects 1.0.9) and let that provider manage it
  until the pod is due to be recreated anyway, then create its successor under 2.0.0.
- **Adopt it with `ignore_changes`.** If the pod has to move to 2.0.0 now, add a `lifecycle` block naming
  every one of the eight the configuration sets, before the first apply. Listing all eight is harmless if
  you are not sure which ones the pod was created with.

  ```hcl
  resource "runpod_pod" "training" {
    # ...
    start_ssh     = true
    start_jupyter = true

    lifecycle {
      ignore_changes = [start_ssh, start_jupyter]
    }
  }
  ```

  The cost: Terraform never enforces those values on this pod. State keeps them empty, so changing one
  later plans nothing at all, neither an update nor a replacement, and the configuration only records what
  the pod was created with. To apply a new value, take the attribute out of `ignore_changes` and accept the
  replacement.

  On a GPU pod, also set `cloud` and `gpu_count` to the values the pod already has. When either is left out
  of the configuration, any other change on the pod plans it as unknown, and both force replacement.
  `desired_state` is one such change: an import leaves it empty, so a configuration that sets it plans an
  in-place update. `env` is another: an import reads the pod's whole environment, so a configuration that
  sets it plans an in-place update that removes every key it leaves out.

Either way, read the first plan before applying it: it must not replace a pod you mean to keep, and the
`# forces replacement` marker names the attribute responsible.

**Same type name, same id format, re-import needed** — the type name does not change and the id is the one
you already have, but the attribute set did change, so remove and import:

- `runpod_pod` — id is the pod id.

  ```
  terraform state rm runpod_pod.training
  terraform import runpod_pod.training pod_abc123
  ```

- `runpod_template` — id is the template id.
- `runpod_network_volume` — id is the volume id.
- `runpod_machine` — id is the machine id. Attribute names are unchanged here, but re-import anyway: the
  resource is a different implementation and importing is cheap.

**Type renamed, re-import needed** — the old address cannot be read at all once 2.0.0 is in the lock file,
because the type no longer exists. `terraform state mv` does not help: the type is part of the address and
the schemas differ. Remove the old address, then import the new one against the same id:

- `runpod_endpoint` becomes `runpod_serverless_endpoint` — id is the endpoint id.

  ```
  terraform state rm runpod_endpoint.inference
  terraform import runpod_serverless_endpoint.inference ep_abc123
  ```

**Type renamed and import unsupported — must be recreated:**

- `runpod_container_registry_auth` becomes `runpod_registry`. The v2 API does not return the username or
  the password, so the resource has no import path. Remove the old address and let the new resource be
  created; the credential gets a **new id**, so update every `registry` reference on pods and endpoints.
  The 1.x credential stays on the account until you delete it in the console.

  ```
  terraform state rm runpod_container_registry_auth.private
  terraform apply
  ```

- `runpod_ecr_delegation` becomes `runpod_registry_delegation`. Same situation: the ARN is not echoed by
  the API, so there is no import path. Remove the old address, apply, and the delegation is recreated with
  a new id. The 1.x delegation stays until deleted in the console.

**Remove from state, then re-declare:**

- `runpod_pod_action` — the resource holds no pod state and cannot be imported. Remove the old address.
  When you re-declare it, **creating the resource runs the action again** — so a `terminate` action you
  re-declare will terminate the pod. If what you wanted was a persistent run state, drop the resource and
  set `runpod_pod.desired_state` instead; if you wanted a one-off, add a `triggers` map so future runs are
  deliberate.

**Remove from state, nothing to import:**

- `runpod_endpoint_job` and `runpod_endpoint_worker` — the resource types are gone.

  ```
  terraform state rm runpod_endpoint_job.warmup
  terraform state rm runpod_endpoint_worker.pinned
  ```

**Resources that keep a stable id across the upgrade** (so the value you pass to `terraform import` is the
one already in your 1.x state): pods, templates, network volumes, Serverless endpoints, machines. The
account SSH keys singleton, new in 2.0.0, imports by the literal id `account`.

**No state action needed:** all data sources. They hold no managed state and re-read on the next plan.
Rename `runpod_container_registry_auths` to `runpod_registries` and `runpod_ecr_delegations` to
`runpod_registry_delegations`, and delete the removed ones (`runpod_endpoint_jobs`,
`runpod_endpoint_job_logs`). The three billing data
sources keep their 1.x names and keep working, deprecated; moving them to the v2 equivalents
(`runpod_billing_pod` to `runpod_pod_billing`, `runpod_billing_network_volume` to
`runpod_network_volume_billing`, `runpod_billing_endpoint` to `runpod_serverless_billing`) is a separate
rewrite (the record shape is different) and is what new configuration should use.

**Final check.** After the state work, run `terraform plan` and expect: no changes for templates, network
volumes, endpoints and machines; for each imported pod, no replacement, and at most an in-place update for
the `env` and `desired_state` an import leaves empty; a create for each registry credential and delegation;
a refresh on the log, worker and billing data sources. A planned pod replacement is either a create-time
attribute the pod paragraph above says to cover or, like any other planned replacement, a configuration
mismatch against what the API reports; the `# forces replacement` marker tells which. Fix the configuration
rather than applying it.
