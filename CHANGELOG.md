# Changelog

## 2.0.0 (unreleased; release candidates 2.0.0-rc.1 on 2026-10-13, 2.0.0-rc.2 on 2026-10-14, 2.0.0-rc.3 on 2026-10-19, 2.0.0-rc.4 on 2026-10-27)

A rebuild of the provider on the Runpod REST v2 API and the generated Go SDK.
The 1.x provider stays on the registry at its published versions; nothing about
it changes. A 1.x configuration will not plan unchanged against 2.0.0; read
the [migration guide](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/migrating-from-1-x)
before pinning `~> 2.0`.

The Go module for this release is `github.com/runpod/terraform-provider-runpod/v2`
and its source lives in the `v2/` directory of this repository. The registry
address is unchanged: `runpod/runpod`.

Runnable configuration ships with the provider as fifteen use-case modules
under `v2/modules/`, each with a generated guide page in the registry docs, an
acceptance test, and a coverage gate that fails the build when a registered
resource or data source is used by none of them. A module is used straight
from this repository at the release tag, for example
`github.com/runpod/terraform-provider-runpod//v2/modules/gpu-pod?ref=v2.0.0`.

### Changes in 2.0.0-rc.4

The provider makes no REST v1 call. What 2.0.0-rc.3 read or wrote through
REST v1 now goes to REST v2 or, where REST v2 has no field, to the GraphQL API
the machine and user shims already use, so the REST v1 retirement on
2026-11-15 does not affect 2.0.0. No resource, data source or attribute is
removed or renamed.

Added:

- `runpod_global_volume` resource and data source: usage-billed storage with
  no data center, on `/v2/global-volumes`. Only `name` is settable and it
  renames in place; `id`, `created_at` and `created_by_user_id` are read back.
  The resource imports by volume id; the data source reads one volume by `id`.
  A destroy of a volume that is still mounted fails with the API's reason.
- `global_volumes` on `runpod_pod`: at most one global volume, as
  `{ volume_id, path }`, sent as the v2 pod's `mounts.global`. It combines with
  `network_volumes` and not with `persistent_volume`, and like a network mount
  it is fixed at create: adding it replaces the pod, and only `path` changes in
  place. The REST v2 specification allows it on CPU pods too; the API's own
  validation decides.
- `global_volumes` on `runpod_serverless_endpoint`: at most one global volume
  id, sent as the v2 endpoint's `mounts.global` and mounted on every worker at
  `/runpod-volume`. GPU endpoints only, never together with network volumes;
  `[]` detaches it.
- A `global-volume` use-case module, with its guide and acceptance test.

Where each REST v1 call went:

- `runpod_billing_pod`, `runpod_billing_network_volume`,
  `runpod_billing_endpoint`: the REST v2 billing routes
  `GET /v2/billing/pods`, `GET /v2/billing/network-volumes` and
  `GET /v2/billing/serverless`, filtered by `podId`, `networkVolumeId` and
  `serverlessId`. (`/v2/billing/endpoints` is spend on Runpod's public
  endpoints and stays behind `runpod_endpoint_billing`.)
- `runpod_billing_pod` with `gpu_type_id`: the GraphQL billing query
  `myself { billing { gpuCloud } }`, which reports GPU pod spend and billed time
  per GPU type and bucket, the grouping REST v1 used for pod billing by default.
- `runpod_pod` `machine_id` and `volume_encrypted`: the GraphQL `pod` query.
- `runpod_template` `docker_entrypoint`: the v2 template's `entrypoint`, sent
  on `POST` and `PATCH /v2/templates` and read back from it.
- `runpod_template` `readme`: the GraphQL `saveTemplate` mutation. The
  mutation resets the fields it is not sent to defaults, so the provider reads
  the stored template first and sends it back with only the readme changed.
- `readme`, `earned`, `is_runpod` and `runtime_in_min` on the
  `runpod_template` resource and data source: the GraphQL `podTemplate` query.
- `runpod_serverless_endpoint` `version` and `workers`:
  `GET /v2/serverless/{id}/workers`, the route `runpod_endpoint_workers`
  reads. `users`: the GraphQL `myself { endpoint(id) { userId } }` query.
- `runpod_billing_endpoint` `time_billed_ms`: the GraphQL billing query
  `myself { billing { serverless(input: { groupBy: ENDPOINT }) } }`, matched to
  each record by endpoint and bucket start.
- `runpod_serverless_endpoint` `gpu_type_priority`, `cpu_flavor_priority`
  (written by a REST v1 patch) and `template_version` (read from the REST v1
  endpoint): no API has them, see below.

Behaviour changes:

- `runpod_pod.volume_encrypted` reads `false` for an unencrypted volume, where
  REST v1 could leave it null: the GraphQL field is non-null. It is null only
  when the GraphQL API does not report the pod or the read fails.
- `runpod_template.docker_entrypoint` is no longer deprecated, and it cannot
  be set together with `args`: the API stores the entrypoint inside `args`, so
  a configuration that sets both fails validation. To set an entrypoint and a
  command together, write `args` in its JSON form, for example
  `{"entrypoint":["/bin/bash","-c"],"cmd":["python -u main.py"]}`. `[]`
  clears the entrypoint.
- `runpod_template.readme` cannot be cleared: `saveTemplate` trims the value
  and ignores an empty one, so `""` or a whitespace-only value is rejected at
  plan time. When the stored template cannot be sent back unchanged (no name,
  disk sizes or category, or a category `saveTemplate` does not accept), no
  mutation is sent and the apply fails with an error that says why, after the
  REST v2 change was applied and saved to state.
- `earned` and `runtime_in_min` on the template resource and data source read
  0, not null, for a template the account does not own.
- `gpu_type_priority` and `cpu_flavor_priority` are not applied: neither REST
  v2 nor the GraphQL API has a setting for the order in which the scheduler
  tries GPU types or CPU flavors. A configured value is kept in state as
  written, never sent, and reported by a warning at plan time. In 2.0.0-rc.3
  setting one failed the apply, because REST v1 rejected the field.
- `runpod_serverless_endpoint.template_version` is always null: neither API
  reports the version of the template an endpoint was built from. A value an
  earlier release wrote to state turns null on the next refresh or apply.
- `runpod_serverless_endpoint.workers`: `pod_id` is the worker's id, because
  REST v2 lists an endpoint's pods as its workers; `last_busy_ms` is always
  null, because no API reports it; and `uptime_ms` is the v2 uptime in whole
  seconds multiplied out. A failed workers read warns and nulls `version` and
  `workers`.
- The three `runpod_billing_*` data sources keep every 1.x argument and record
  field. `amount` is the v2 record's total and `time` the start of its bucket.
  - `runpod_billing_endpoint`'s `time_billed_ms` comes from the GraphQL API's
    serverless billing for the same endpoint and bucket, at whole-second
    precision. It is null for yearly buckets, for hourly buckets more than 13
    days old and other buckets more than 13 months old (that API reports those
    only in coarser buckets), for a bucket it has no row for, and when the
    read fails, which warns.
  - `runpod_billing_pod` with `gpu_type_id` answers the way REST v1 did, one
    record per bucket for that GPU type, from the GraphQL GPU pod billing:
    `gpu_type_id`, `amount`, `time` and `time_billed_ms` filled, `pod_id`,
    `endpoint_id` and `disk_space_billed_gb` null. Hourly buckets more than 13
    days old and other buckets more than 13 months old are not reported, and a
    yearly bucket sums the monthly rows. Setting `gpu_type_id` together with
    `pod_id` is an error at plan time: no API filters by both.
  - Without `gpu_type_id`, `runpod_billing_pod` reports one record per pod and
    bucket from REST v2, where REST v1 grouped by GPU type: `pod_id` is filled,
    and `gpu_type_id`, `endpoint_id`, `disk_space_billed_gb` and
    `time_billed_ms` are null, because no API reports them per pod.
  - `runpod_billing_network_volume` reports one record per volume and bucket
    from REST v2, where REST v1 reported the whole account (its route took no
    volume filter and returned no volume id): `network_volume_id` is filled and
    the filter applies, and `disk_space_billed_gb` and `time_billed_ms` are
    null. The GraphQL billing API reports billed network storage only for the
    whole account, not per volume.
  - `end_time` takes an RFC 3339 timestamp, a datetime without a zone or a
    date, the last two read as UTC; anything else is an error.
  - REST v2 takes a start and an end time only as a pair, so `end_time` goes
    out with a start 30 days before it. The window is the 30 days before
    `end_time`, or before the current time when it is unset.
- A GraphQL read that fails warns and leaves the attributes it feeds null, as
  the REST v1 reads did. An object the GraphQL API does not report (a pod or an
  endpoint it does not know, a template the account cannot see) leaves them
  null without a warning.
- Provider configuration: `rest_v1_url` is deprecated and ignored, and
  `RUNPOD_REST_V1_URL` and `RUNPOD_BASE_URL` are no longer read. `graphql_url`
  now also serves the deprecated pod, template and endpoint attributes.
- `runpod_clusters` and `runpod_network_volumes` follow the cursor pagination
  the API now applies to both lists, so they still return every cluster and
  every volume rather than the first page.
- Modules: the legacy module drops its `gpu_type_id` variable and the
  `pod_time_billed_ms`, `pod_gpu_types` and `network_volume_disk_gb` outputs;
  its pod and volume billing reads are unfiltered, and those values have no
  per-pod or per-volume source. The template module's `docker_entrypoint` is no
  longer deprecated and cannot be set together with `args`.

Upgrading from 2.0.0-rc.3: remove `rest_v1_url` from the provider block,
`gpu_type_priority` and `cpu_flavor_priority` from Serverless endpoints,
`gpu_type_id` or `pod_id` from a `runpod_billing_pod` that sets both,
`readme = ""` from templates, and
either `args` or `docker_entrypoint` from a template that sets both (or fold
both into `args`). A configuration that tests `volume_encrypted == null`, or
that reads `template_version`, `workers[].last_busy_ms` or the billing fields
above, needs to account for the new values.

### Changes in 2.0.0-rc.3

Documentation and release tooling only; no provider behaviour changes. The Go
changes are description strings.

- The release the docs point at is one line, `RELEASE` in `v2/Taskfile.yml`:
  `task docs` rewrites every module `?ref=`, module README link, provider pin
  and module version pin from it, the CI drift gate fails when they are out of
  date, and a tag whose number differs from `RELEASE` does not release. The
  local build carries the same number, so it reports `2.0.0-rc.3-dev`.
- The modules pin the release their guide documents, an exact pin while it is
  a candidate, so `2.0.0-rc.3` installs them from the registry.
- Registry index: one pointer to the agent guide instead of the agent section,
  Example Usage creates a template, and the note under it says why a release
  candidate needs an exact pin.
- The fourteen module guides sit under one "Use-case modules" group; the agent
  and migration guides have no group and list first.
- Agent guide: the first step is optional, so an agent without the Runpod
  skills can follow the steps; the version step names the pin to use while
  only release candidates exist, the import section covers clusters and the
  SSH keys id, and the leftover check lists templates, clusters and registry
  credentials too.
- REST v1 is retired on 2026-11-15: the migration guide, the legacy guide, the
  v2 README, the changelog and the shim descriptions say what stops working
  then; `gpu_type_priority` and `cpu_flavor_priority` are documented as
  rejected by REST v1, and the Serverless modules say to leave them null.
- Examples: the pod and Serverless snippets declare their own template, a pod
  with a network volume sets `data_center_ids`, the deprecated pod outputs and
  the template's REST v1 writes are gone, and the pod Import section lists the
  create-time settings an import leaves empty.
- The legacy module README links the migration guide at its real path; the v2
  README no longer names "stage".
- The 1.x `examples/` at the repository root, a committed state file and a
  developer-override config are removed, `*.tfstate` is ignored, and the root
  README says the remaining root `*.md` files describe 1.x.

### Changes in 2.0.0-rc.2

No provider behaviour changes; the Go changes are description strings.

- New guide, "Using the provider from an AI agent", linked from the registry
  index and README.md; the registry index and every resource and data-source
  page carry a pointer to it.
- The machine, user and three `runpod_billing_*` pages are grouped under
  Deprecated in the registry navigation.
- `runpod_registry`, `runpod_registry_delegation` and `runpod_pod_action` say
  they cannot be imported; a re-declared pod action runs again.
- The Serverless endpoint example no longer sets the deprecated attributes or
  outputs.
- Local builds report `2.0.0-dev` in the User-Agent, so the API's request data
  tells the project's own runs from users'.
- Test files: legacy no longer asserts outputs the module dropped, registry
  declares the variables its run block reads, and the pod tests declare
  `gpu_type_id` as a variable. Running the tests needs Terraform 1.13; the
  provider and the modules still need 1.6.

### Provider configuration

- `api_key`: Runpod API key, or the `RUNPOD_API_KEY` environment variable.
- `base_url`: REST v2 base URL, or `RUNPOD_API_BASE_URL`; set it only to target
  a non-production Runpod environment.
- `graphql_url`: GraphQL endpoint, or `RUNPOD_GRAPHQL_URL`. Used only by the
  deprecated machine resource and data sources, the deprecated `runpod_user`
  data source, and the deprecated attributes on `runpod_pod`,
  `runpod_template` (resource and data source) and
  `runpod_serverless_endpoint` that REST v2 does not report.
- `rest_v1_url`: deprecated and ignored; the provider makes no REST v1 call.
  It is kept so a configuration that sets it still validates.
  `RUNPOD_REST_V1_URL` and `RUNPOD_BASE_URL` are not read.

Every request carries the User-Agent
`terraform-provider-runpod/<version> (terraform <version>; go<version>) runpod-go-sdk/<version>`.
The deprecated GraphQL calls send the value 1.x sent,
`Terraform/RunPodProvider`, followed by the same string.

### Resources

- `runpod_pod`
- `runpod_pod_action`
- `runpod_network_volume`
- `runpod_global_volume`
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
- `runpod_global_volume`
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

These ship so a 1.x configuration keeps planning. Each is either a shim over
the GraphQL API, because REST v2 has no equivalent route or field yet, or a
1.x shape kept over REST v2, and each is removed in the first release after
REST v2 covers it or the 1.x shape is dropped. Plans that use them emit a
deprecation warning. None of them calls REST v1.

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
  `runpod_billing_endpoint` data sources. These keep the 1.x billing data
  sources' arguments and record fields, so a 1.x configuration keeps planning,
  and fill them from the REST v2 billing routes. The endpoint record's
  `time_billed_ms` comes from the GraphQL API's serverless billing, and a
  `gpu_type_id` read of the pod data source comes from the GraphQL GPU pod
  billing, one record per GPU type and bucket as REST v1 grouped it. The
  per-record fields no API reports per pod or per volume always read null:
  `time_billed_ms` and `disk_space_billed_gb` on the pod and network volume
  records, and the pod record's `gpu_type_id` and `endpoint_id`, outside a
  `gpu_type_id` read. New configurations
  should use `runpod_pod_billing`, `runpod_network_volume_billing` and
  `runpod_serverless_billing`, which read the same routes with the cost
  components broken out. Note that `runpod_endpoint_billing` in 2.0.0 is a
  different thing, spend on Runpod's own public endpoints. One behaviour
  change: a failed read is an error rather than an empty result, because the
  records are these data sources' whole body.
- `registries[].username` on the `runpod_registries` data source. It is kept
  so a 1.x configuration that references it still plans, and it always reads
  null: REST v2 treats the username as write-only and no other supported API
  reports it, so the provider sends no request for it.
- Attributes kept on three resources so 1.x state and outputs survive the
  upgrade. A read-back that nothing configures warns and reads null if its
  read fails; an attribute the configuration sets is a write, and a failure
  there fails the apply:
  - `runpod_pod`: `machine_id` and `volume_encrypted`: GraphQL read-backs,
    read-only, because the 1.x provider never sent them on a create either.
    `volume_encrypted` is `false` for an unencrypted volume.
  - `runpod_serverless_endpoint`: `gpu_type_priority` and `cpu_flavor_priority`
    are not applied, because no API has a setting for that order: a configured
    value is kept in state, never sent, and warned about at plan time.
    `template_version` is always null, because no API reports it. `users` is a
    GraphQL read-back. `version` and `workers` are read from the REST v2
    workers listing, and `runpod_endpoint_workers` is the supported view of the
    same workers. `compute_type` is deprecated for a different reason: it
    restates what `gpu_pools` or `cpu` already says, so it is derived from the
    v2 response with no call to any other API, and a value contradicting the
    block is rejected.
  - `runpod_template` resource: `readme` is written through the GraphQL
    `saveTemplate` mutation on create and update when the configuration sets
    it, and read back; it cannot be cleared, so `""` is rejected at plan time.
    `earned`, `is_runpod` and `runtime_in_min` are GraphQL read-backs. On the
    `runpod_template` data source the same four (`readme`, `earned`,
    `is_runpod`, `runtime_in_min`) are read-only GraphQL read-backs.

Not a shim, despite looking like one: `runpod_pod_action.status`. 1.x read it
off a GraphQL mutation result and the v2 action response carries the same
thing, so it survives on REST v2 with no deprecation. It is the status at the
moment the action was accepted, not a live value, and `terminate` answers with
no body, so it leaves this null.

Not shims either: `docker_entrypoint` and `docker_start_cmd` on `runpod_pod`,
and `docker_entrypoint` on the `runpod_template` resource and data source.
They are read from the v2 pod's `entrypoint` and `cmd` and the v2 template's
`entrypoint`, which the API decodes from `args`, and are not deprecated. On
the pod both are now read-only: 1.0.8 accepted a value but never sent it, so a
configuration that sets one fails validation. Remove it and put the command in
`args`. On the template resource `docker_entrypoint` is sent as the v2
template's `entrypoint` and cannot be set together with `args`.

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
  The deprecated `runpod_serverless_endpoint.workers` snapshot keeps both
  attributes: `pod_id` is the worker's id, and `last_busy_ms` is always null.
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
