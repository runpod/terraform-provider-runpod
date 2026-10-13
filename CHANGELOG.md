# Changelog

## 2.0.0

A rebuild of the provider on the Runpod REST v2 API and the generated Go SDK.
The 1.x provider stays on the registry at its published versions; nothing about
it changes. A 1.x configuration will not plan unchanged against 2.0.0; read
the migration guide before pinning `~> 2.0`.

The Go module for this release is `github.com/runpod/terraform-provider-runpod/v2`
and its source lives in the `v2/` directory of this repository. The registry
address is unchanged: `runpod/runpod`.

Runnable configuration ships with the provider as fourteen use-case modules
under `v2/modules/`, each with a generated guide page in the registry docs, an
acceptance test, and a coverage gate that fails the build when a registered
resource or data source is used by none of them. A module is used straight
from this repository at the release tag, for example
`github.com/runpod/terraform-provider-runpod//v2/modules/gpu-pod?ref=v2.0.0`.

### Provider configuration

- `api_key`: Runpod API key, or the `RUNPOD_API_KEY` environment variable.
- `base_url`: REST v2 base URL, or `RUNPOD_API_BASE_URL`; for stage or
  self-hosted targets.
- `graphql_url`: GraphQL endpoint, or `RUNPOD_GRAPHQL_URL`. Used only by the
  deprecated machine resource and data sources and by the deprecated
  `runpod_user` data source.
- `rest_v1_url`: REST v1 base URL, or `RUNPOD_REST_V1_URL`, then
  `RUNPOD_BASE_URL` (the 1.x name for this host). Used only by the three 1.x
  billing data sources and the deprecated attributes on `runpod_pod`, `runpod_template` (resource and data source) and
  `runpod_serverless_endpoint`.

Every request carries the User-Agent
`terraform-provider-runpod/<version> (terraform <version>; go<version>) runpod-go-sdk/<version>`.
The deprecated GraphQL and REST v1 calls send the value 1.x sent,
`Terraform/RunPodProvider`, followed by the same string.

### Resources

- `runpod_pod`
- `runpod_pod_action`
- `runpod_network_volume`
- `runpod_template`
- `runpod_registry`
- `runpod_registry_delegation`
- `runpod_serverless_endpoint`
- `runpod_cluster`
- `runpod_account_ssh_keys`
- `runpod_machine`, deprecated, see below.

### Data sources

Catalog and placement:

- `runpod_gpu_types`, `runpod_gpu_type`
- `runpod_cpu_types`, `runpod_cpu_type`
- `runpod_data_centers`, `runpod_data_center`
- `runpod_catalog_templates`

Inventory:

- `runpod_pods`, `runpod_pod`
- `runpod_templates`, `runpod_template`
- `runpod_network_volumes`
- `runpod_serverless_endpoints`
- `runpod_registries`
- `runpod_registry_delegations`
- `runpod_clusters`, `runpod_cluster`, `runpod_cluster_pods`

Serverless detail:

- `runpod_endpoint_workers`, `runpod_endpoint_worker_logs`
- `runpod_endpoint_builds`, `runpod_endpoint_build`
- `runpod_endpoint_releases`

Logs:

- `runpod_pod_logs`

Billing, time-bucketed with cost components broken out:

- `runpod_billing`
- `runpod_pod_billing`
- `runpod_serverless_billing`
- `runpod_endpoint_billing`
- `runpod_network_volume_billing`
- `runpod_cluster_billing`

The Serverless platform fee is part of the GPU and CPU amounts. The API's
separate fee field is deprecated and always 0, so no data source reports it.

Deprecated, see below:

- `runpod_machine`, `runpod_machines`
- `runpod_user`
- `runpod_billing_pod`, `runpod_billing_network_volume`, `runpod_billing_endpoint`

### Deprecated in 2.0.0

These ship and work. Each is a shim over an older API because REST v2 has no
equivalent route yet, and each is removed in the first release after REST v2
covers it. Plans that use them emit a deprecation warning.

- `runpod_machine` resource and the `runpod_machine` / `runpod_machines` data
  sources. Machines have no REST v2 route; these call the GraphQL API. Only
  `name` and `bid_price_per_gpu` are settable (the create mutation takes no
  other input), and every other attribute reports what the API assigned. Two
  1.x mutations that never matched the GraphQL schema are corrected: a rename
  sends `machineId`, and a destroy sends `machineDelete(input: { machineId })`.
  A machine the API no longer reports is removed from state.
- `runpod_user` data source. There is no REST v2 account identity route, and
  the REST v1 `/user` route 1.x read is gone, so this reads the GraphQL
  `myself` query. Every attribute keeps its name, but only `id` and `email`
  have a source there; the others read null.
- `runpod_billing_pod`, `runpod_billing_network_volume` and
  `runpod_billing_endpoint` data sources. These reproduce the 1.x billing data
  sources attribute for attribute, off the REST v1 billing routes, so a 1.x
  configuration keeps planning. They stay only for the per-record fields REST
  v2 billing does not report yet: `time_billed_ms`, `disk_space_billed_gb`, the
  pod record's `endpoint_id`, and the `gpu_type_id` filter. New configurations
  should use `runpod_pod_billing`, `runpod_network_volume_billing` and
  `runpod_serverless_billing`. Note that `runpod_endpoint_billing` in 2.0.0 is
  a different thing, spend on Runpod's own public endpoints. One behaviour
  change: a failed REST v1 read is an error rather than an empty result,
  because the records are these data sources' whole body.
- `registries[].username` on the `runpod_registries` data source. It is kept
  so a 1.x configuration that references it still plans, and it always reads
  null: REST v2 treats the username as write-only and no other supported API
  reports it, so the provider sends no request for it.
- Attributes kept on three resources so 1.x state and outputs survive the
  upgrade. A read-back that nothing configures warns and reads null if the old
  host does not answer; an attribute the configuration sets is a write, and a
  failure there fails the apply:
  - `runpod_pod`: `machine_id` and `volume_encrypted`: REST v1 read-backs,
    read-only, because the 1.x provider never sent them on a create either.
  - `runpod_serverless_endpoint`: `gpu_type_priority` and `cpu_flavor_priority`
    go out as a REST v1 patch on create and on every update that sets them, and
    are never read back, exactly as in 1.x; `template_version`, `users`,
    `version` and `workers` are REST v1 read-backs, and
    `runpod_endpoint_workers` is the supported view of the worker snapshot.
    `compute_type` is deprecated for a different reason: it restates what
    `gpu_pools` or `cpu` already says, so it is derived from the v2 response
    with no call to any older API, and a value contradicting the block is
    rejected.
  - `runpod_template` resource: `docker_entrypoint` and `readme` go out as a
    REST v1 patch on create and update and are read back from it; `earned`,
    `is_runpod` and `runtime_in_min` are read-backs. On the `runpod_template`
    data source the same four (`readme`, `earned`, `is_runpod`,
    `runtime_in_min`) are read-only REST v1 read-backs; its
    `docker_entrypoint` is read from the v2 template.

Not a shim, despite looking like one: `runpod_pod_action.status`. 1.x read it
off a GraphQL mutation result and the v2 action response carries the same
thing, so it survives on REST v2 with no deprecation. It is the status at the
moment the action was accepted, not a live value, and `terminate` answers with
no body, so it leaves this null.

Not shims either: `docker_entrypoint` and `docker_start_cmd` on `runpod_pod`,
and `docker_entrypoint` on the `runpod_template` data source. They are read
from the v2 pod's `entrypoint` and `cmd` and the v2 template's `entrypoint`,
which the API decodes from `args`, and are not deprecated. On the pod both are
now read-only: 1.0.8 accepted a value but never sent it, so a configuration
that sets one fails validation. Remove it and put the command in `args`.

### Removed

Everything below was declared by the 1.x provider and never worked against a
live account. None of it has a replacement, because the platform behaviour it
described does not exist. Anything 1.x genuinely did is in the deprecated list
above. The line between the two is what the last working 1.x release, 1.0.8,
actually put on the wire. 1.0.9 is not the baseline: as published it cannot
serve its own schema and nine of its ten resources panic during provider
configuration, so nothing in it ever ran.

- Spot and interruptible pods: the `type` attribute on `runpod_pod`, which took
  `ON_DEMAND` or `SPOT`, together with `interruptible` and `bid_per_gpu`. The
  platform retired spot pods, so there is nothing left for these to select.
- Four attributes that were schema-only in every 1.x release: declared, never
  sent, never read back: `stop_after`, `terminate_after`, `volume_key` and the
  single `port` attribute on `runpod_pod`. REST v2 has no scheduled stop or
  terminate and no volume encryption key either. Use `runpod_pod.desired_state`
  or `runpod_pod_action` with `stop` or `terminate`, and declare ports as a
  list. Two more 1.x schema-only attributes, `cluster_ip` and `locked`, keep
  their names and are now filled for real from the v2 read.
- `desired_status`, `machine_id` and `machine_type` on the `runpod_pod` data
  source. A defect in the 1.x provider failed that data source's state write on
  every read, whatever the API returned, so no configuration can have depended
  on them. 2.0.0 fixes the data source; the `runpod_pod` resource reports
  `machine_id` as a deprecated read-back.
- `cloud_type` on the endpoint, and `password`, `created_at` and `updated_at`
  on the registry list. These exist only in 1.0.9.
- The `runpod_endpoint_job` and `runpod_endpoint_worker` resources, and the
  `runpod_endpoint_jobs` and `runpod_endpoint_job_logs` data sources. All four
  are 1.0.9-only and call job and worker subroutes that were cancelled before
  they shipped. Jobs and workers are queue and scheduler state, not resources
  Terraform can own; jobs go straight to the Serverless runtime API, whose URLs
  the endpoint reports. `runpod_endpoint_workers` and
  `runpod_endpoint_worker_logs` remain as reads.
- Machine pinning on `runpod_pod`. `machine_id` was an input in 1.x, but no
  1.x create ever sent it, and the v2 create has no machine parameter. It is
  now the deprecated read-only read-back above; constrain placement with
  `data_center_ids` and the per-GPU RAM and vCPU floors.

Also gone, because the v2 API does not return them:

- `gpu_types[].cuda_cores` and `gpu_types[].manufacturer` on `runpod_gpu_types`.
- `ecr_delegations[].resource` on the delegations list: the API does not echo
  the delegated ARN.
- The synthetic `id` on `runpod_endpoint_workers` and
  `runpod_endpoint_worker_logs`.
- `workers[].pod_id` and `workers[].last_busy_ms` on `runpod_endpoint_workers`.
  The deprecated `runpod_serverless_endpoint.workers` snapshot still carries
  both.
- The `ALL` value of the pod's cloud tier: `cloud` takes `SECURE` or
  `COMMUNITY`.

### Renamed

The 1.x name is on the left. There is no automatic state migration; move the
resource in your configuration and re-import, or let Terraform replace it. The
migration guide has the attribute-by-attribute tables.

Types:

- `runpod_endpoint` is now `runpod_serverless_endpoint`.
- `runpod_container_registry_auth` is now `runpod_registry`, and
  `runpod_container_registry_auths` is now `runpod_registries`, whose list
  attribute `container_registry_auths` is now `registries`.
- `runpod_ecr_delegation` is now `runpod_registry_delegation`, and
  `runpod_ecr_delegations` is now `runpod_registry_delegations`, whose list
  attribute `ecr_delegations` is now `delegations`.

Attributes:

- `runpod_pod` resource: `image_name` is now `image`, `docker_args` is `args`,
  `container_disk_in_gb` is `disk`, `cloud_type` is `cloud` and `cost_per_hr`
  is `cost`. `volume_in_gb` and `volume_mount_path` are the `persistent_volume`
  block, and `network_volume_id` and `network_volume_ids` are the
  `network_volumes` list of `{ volume_id, path }`.
- `runpod_pod` data source: the same renames, plus `template_id` is now
  `template`, `memory_in_gb` is split into `gpu_memory` and `cpu_memory`, and
  the persistent volume is under `mounts.persistent`.
- `runpod_serverless_endpoint`: `image_name` is now `image`,
  `container_disk_in_gb` is `disk`, `scaler_type` is `scaling_type`,
  `scaler_value` is `scaling_value` (now required with it),
  `execution_timeout_ms` is `timeout` (still milliseconds), and
  `network_volume_id` and `network_volume_ids` are `network_volumes`. GPU
  selection moved from GPU types to Serverless pools: `gpu_type_id` and
  `gpu_type_ids` are replaced by `gpu_pools`, with `excluded_types` to subtract
  GPU types from them. `cpu_flavor_ids` and `vcpu_count` are the `cpu` block's
  `cpu_type_id` and `vcpu_count`.
- `runpod_template` resource and data source: `image_name` is now `image`,
  `container_disk_in_gb` is `disk`, `is_public` is `public`, `is_serverless`
  is `serverless`, `container_registry_auth_id` is `registry`, and the
  `docker_start_cmd` list is the `args` string. `volume_in_gb` and
  `volume_mount_path` are the `persistent_volume` block, or
  `mounts.persistent` on the data source.
- `runpod_network_volume`: `data_center_id` is now `data_center`, and
  `storage_tier` is `type`.
- `runpod_gpu_types`: `display_name` is now `name`, `memory_in_gb` is
  `memory`, `secure_cloud` is `secure`, `secure_price` is `price_secure` and
  `community_price` is `price_community`.
- `runpod_data_centers`: `location` is now `region`.
- `runpod_endpoint_workers`: `start_time` is now `started_at`, and `uptime_ms`
  is `uptime_seconds`, in seconds.
- `runpod_endpoint_worker_logs`: `logs` is now `lines`.

### Changed types

A configuration that sets one of these needs editing; none converts.

- `runpod_pod` `ports` was a single string in 1.x and is a list of strings in
  2.0.0, on the resource and the data source.
- `runpod_pod` `env` was a list of `"KEY=VALUE"` strings in 1.x and is a map of
  string in 2.0.0, on the resource and the data source.
- `runpod_pod` `vcpu_count` was computed in 1.x and is an input for CPU pods in
  2.0.0. The `runpod_pod` data source reports the allocated count as
  `gpu_vcpu_count` or `cpu_vcpu_count`.
- `runpod_serverless_endpoint` `flashboot` was a boolean in 1.x and is a string
  in 2.0.0: `OFF`, `FLASHBOOT` or `PRIORITY_FLASHBOOT`.
- `runpod_endpoint_worker_logs` `lines` is a list of line strings where 1.x
  `logs` was one string; `join` gives the old shape.

## 1.0.9 and earlier

See the release notes on the GitHub releases page. The 1.x provider remains
published at `runpod/runpod` and its tags are not withdrawn.
