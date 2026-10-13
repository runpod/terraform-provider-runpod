# Terraform Provider for Runpod

A Terraform provider for Runpod. It manages pods (GPU and CPU), Serverless
endpoints, templates, network volumes, clusters, account SSH keys and
container registry credentials through the v2 REST API. Built on the
Terraform Plugin Framework; all API access goes through the generated
`runpod-go-sdk` Go client. A small set of deprecated compatibility shims still
calls the GraphQL and REST v1 APIs, through that SDK's deprecated `legacy`
package, because REST v2 has no route for what they carry: the machine surface
and the registry usernames on GraphQL, and the account read, the three 1.x
billing data sources and a handful of attributes on pods, templates and
Serverless endpoints on REST v1. They exist so a 1.x configuration keeps
working; each warns on every plan and is removed once REST v2 reports the same
thing.

## Requirements

- Terraform 1.6 or higher
- Go 1.25 or higher (for development)
- [Task](https://taskfile.dev) (for development)
- Python 3 (for the module coverage gate and for `task docs`, which re-keys the
  provider schema before rendering)
- A Runpod API key

## Using the provider

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

The provider reads `RUNPOD_API_KEY` from the environment, or takes `api_key`
explicitly. `RUNPOD_API_BASE_URL` (or `base_url`) points it at another
environment, e.g. stage.

Two more arguments exist only for the deprecated shims and are ignored by
everything on REST v2: `graphql_url` (or `RUNPOD_GRAPHQL_URL`) for the machine
resource and data sources and the registry usernames, and `rest_v1_url` (or
`RUNPOD_REST_V1_URL`, then `RUNPOD_BASE_URL`, the 1.x name for that host) for
the user data source, the 1.x billing data sources and the deprecated
attributes on pods, templates and Serverless endpoints.

```hcl
data "runpod_gpu_types" "all" {}

resource "runpod_pod" "training" {
  name        = "training-pod"
  image       = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  gpu_type_id = "NVIDIA GeForce RTX 4090"
  gpu_count   = 1
  cloud       = "SECURE"
  disk        = 50

  env = {
    MODEL_NAME = "llama-3"
  }

  ports = ["8888/http", "22/tcp"]
}
```

### Resources

| Resource | Purpose |
|---|---|
| `runpod_pod` | Pod lifecycle: image or template, GPU or CPU config, CUDA and placement constraints, cloud tier, disk, persistent and network volume mounts, SSH and Jupyter start flags, env, ports, data center pinning, global networking, private registry, managed run state (`desired_state`, reconciled on every refresh) |
| `runpod_serverless_endpoint` | Serverless endpoints: GPU pools or CPU configurations, template, FlashBoot, CUDA constraints, queue/load-balancer routing with worker ports, autoscaling, worker bounds, private registry |
| `runpod_cluster` | Multi-node clusters (training, Slurm, Ray, application) and their pods |
| `runpod_account_ssh_keys` | The account's SSH public keys, managed as one replaced document |
| `runpod_pod_action` | Imperative start / stop / restart / terminate of an existing pod, re-run on `triggers` change |
| `runpod_template` | Reusable pod/endpoint configuration templates |
| `runpod_network_volume` | Persistent network storage volumes |
| `runpod_registry` | Container registry credentials for private images |
| `runpod_registry_delegation` | AWS ECR pull delegations |
| `runpod_machine` | Deprecated shim over the GraphQL API; removed when a REST v2 machines route exists. Only `name` and `bid_price_per_gpu` are settable |

Deprecated attributes preserved from the 1.x provider, each marked deprecated
in the schema and served from an older API because REST v2 has no field for it
yet:

| Resource | Deprecated attributes | Where they come from |
|---|---|---|
| `runpod_pod` | `machine_id`, `volume_encrypted` | REST v1 read-back after the v2 read; read-only, as they effectively were in 1.x |
| `runpod_serverless_endpoint` | `gpu_type_priority`, `cpu_flavor_priority` | REST v1 patch on create and update when set, never read back, as in 1.x |
| `runpod_serverless_endpoint` | `template_version`, `users`, `version`, `workers` | REST v1 read-back; `runpod_endpoint_workers` is the supported view of the worker snapshot |
| `runpod_serverless_endpoint` | `compute_type` | Derived from the v2 response, with no call to any older API; a value contradicting `gpu_pools` or `cpu` is rejected |
| `runpod_template` | `docker_entrypoint`, `readme` | REST v1 patch on create and update, read back from it |
| `runpod_template` | `earned`, `is_runpod`, `runtime_in_min` | REST v1 read-back |

A deprecated attribute nothing configured is a read: if the older API does not
answer, the plan warns and the attribute reads null. One the configuration sets
is a write, and a failure there fails the apply.

`runpod_pod`, `runpod_network_volume`, `runpod_template`,
`runpod_serverless_endpoint`, `runpod_cluster` and `runpod_machine` support
`terraform import` by resource id; `runpod_account_ssh_keys` imports by the
literal id `account`. `runpod_registry` and `runpod_registry_delegation` do
not: the API never returns the credential secret or the delegated ARN, so an
imported resource could not be reconciled against configuration.
`runpod_pod_action` is imperative and has nothing to import.

### Data sources

Catalog: `runpod_gpu_types`, `runpod_cpu_types`, `runpod_data_centers`,
`runpod_catalog_templates`, plus by-id lookups `runpod_gpu_type`,
`runpod_cpu_type`, `runpod_data_center`.
Account listings: `runpod_pods`, `runpod_templates`, `runpod_network_volumes`,
`runpod_serverless_endpoints`, `runpod_registries`,
`runpod_registry_delegations`, `runpod_clusters`.
By id: `runpod_pod`, `runpod_template`, `runpod_cluster`, `runpod_cluster_pods`.
Endpoint detail: `runpod_endpoint_workers`, `runpod_endpoint_builds`,
`runpod_endpoint_build`, `runpod_endpoint_releases`.
Logs (bounded reads of the streaming routes): `runpod_pod_logs`,
`runpod_endpoint_worker_logs`.
Billing: `runpod_billing`, `runpod_pod_billing`, `runpod_serverless_billing`,
`runpod_endpoint_billing`, `runpod_network_volume_billing`,
`runpod_cluster_billing`.
Deprecated shims: `runpod_machine`, `runpod_machines`, `runpod_user` (GraphQL;
REST v1 `/user` is gone, so the account read is the `myself` query and only
`id` and `email` are served), and the three 1.x billing data sources `runpod_billing_pod`,
`runpod_billing_network_volume`, `runpod_billing_endpoint` (REST v1), which
keep their 1.x arguments and record fields and stay only for the per-record
values v2 billing does not carry yet: `time_billed_ms`,
`disk_space_billed_gb`, the pod record's `endpoint_id` and the `gpu_type_id`
filter. New configuration belongs on the v2 billing data sources above.
Two supported data sources also carry deprecated attributes: `runpod_template`
(`readme`, `earned`, `is_runpod`, `runtime_in_min`, on REST v1) and
`runpod_registries` (`username`, on GraphQL).

Working configurations live in [modules/](modules/): fourteen use-case
modules, each with variables, outputs, a README and an acceptance test. Every
registered resource and data source above is used by at least one of them, and
`task validate-tf` fails when that stops being true. A module is usable straight from this repository:

```hcl
module "training_pod" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/gpu-pod?ref=v2.0.0"

  name        = "training"
  gpu_type_id = "NVIDIA GeForce RTX 4090"
  disk        = 50
}
```

[modules/README.md](modules/README.md) lists all fourteen with what each costs
to run. The per-type snippets under `examples/resources/` and
`examples/data-sources/` are documentation sources for the registry pages, not
modules.

## How it works

The provider is Terraform Plugin Framework code in `internal/provider/`. Every
API call goes through `runpod-go-sdk`, the Go client generated from the Runpod
OpenAPI specification. The provider adds no HTTP of its own. The deprecated
shims are no exception: their GraphQL and REST v1 calls go through
`github.com/runpod/go-sdk/v2/legacy`, the SDK's hand-written deprecated
package, which is removed once REST v2 carries the same fields. Each resource
and data source is one file: schema, CRUD, and (where the API supports it)
import. The API↔state conversion lives apart from the plugin protocol in
`internal/provider/mapping.go`, which is what the unit tests exercise.

Documented enums and numeric bounds carry schema validators, so a bad `cloud`
tier or an out-of-range volume `size` fails in `terraform validate` and
`terraform plan` — offline, before any API call.

Everything under `docs/` at the repository root, one level up from this
directory, where the Terraform Registry reads it, is generated by tfplugindocs
from the built provider's schema and is never edited by hand. Attribute documentation is the
`Description` field on the schema attribute in `internal/provider/*.go`;
change it there and run `task docs` to re-render. Usage examples on those
pages come from `examples/resources/<type>/resource.tf` and
`examples/data-sources/<type>/data-source.tf`, the import sections from
`examples/resources/<type>/import.sh`, and the page shape from
`templates/index.md.tmpl`, `templates/resources.md.tmpl` and
`templates/data-sources.md.tmpl`. `docs/guides/` is one page per use-case
module, rendered from `templates/guides/<module>.md.tmpl`, which embeds the
module's own `main.tf`.

The input and output tables inside each module README, between its
`BEGIN_TF_DOCS` and `END_TF_DOCS` markers, are generated too, by
terraform-docs from the module's `.tf` files, configured in
`.terraform-docs.yml`. The prose above the markers is hand-written.

## Repository layout

    main.go                  provider entry point
    internal/provider/       schemas, CRUD, and API↔state mapping
    modules/                 one runnable use-case module per thing you would
                             do with Runpod, each with its own README
    examples/                the documentation snippet tree rendered into the
                             Example Usage sections of docs/
    templates/               tfplugindocs templates: the index, the generic
                             resource and data-source pages, and guides/ —
                             one guide template per module
    tests/                   terraform test files that apply the modules
    scripts/                 module_coverage.py, the coverage gate
    .terraform-docs.yml      terraform-docs config for the module READMEs
    .golangci.yaml           golangci-lint config, the linter set Runpod's Go repos use
    Taskfile.yml             build, test, lint, validate, and docs tasks

At the repository root, next to the 1.x provider:

    docs/                    generated registry documentation
    .github/workflows/       CI: lint, unit tests with the race detector, offline
                             validation; the release build for v2.* tags
    .goreleaser.v2.yml       the release build of this module
    CHANGELOG.md             release notes

## Development

Credentials go in `.env` (see `.env.example`); every task loads it. Run the
tasks from this directory.

```bash
task build        # build into a local filesystem mirror + artifacts/dev.tfrc
task test         # unit tests (offline)
task lint         # golangci-lint over the module (.golangci.yaml)
task validate-tf  # validate, format, coverage, and docs checks (offline)
task test-tf      # terraform test over the modules against the live API
task docs         # registry docs into ../docs/ + the module README tables
task all          # build + test + lint + validate-tf
```

`task validate-tf` is the offline gate and runs four checks beyond
`terraform validate` of `tests/` and of every module: `terraform fmt -check`
over `modules/` and `tests/`; `scripts/module_coverage.py`, which fails when a
resource or data source the built provider registers is used by no module;
`terraform-docs --output-check`, which fails when a module README's generated
tables are stale; and a check that every module has a
`templates/guides/<module>.md.tmpl`. A new resource therefore cannot ship
without a module that uses it, and a new module cannot ship without its guide
page.

`task build` installs the provider under `artifacts/provider-mirror` and writes
`artifacts/dev.tfrc`, a CLI config that makes terraform install from that
mirror. The terraform-facing tasks set `TF_CLI_CONFIG_FILE` to it themselves;
to run a module manually against the local build:

```bash
export TF_CLI_CONFIG_FILE=$PWD/artifacts/dev.tfrc
terraform -chdir=modules/catalog-discovery init
terraform -chdir=modules/catalog-discovery apply
```

## Testing

Unit tests (`task test`) construct every resource and data-source schema
(with the framework's own implementation validation and a check that no
attribute ships without a description) and cover the API↔state mapping. They
need no network access and no API key.

Acceptance tests (`task test-tf`) are the use-case modules: each file in
`tests/*.tftest.hcl` applies `modules/<module>` via `terraform test`, asserts
on its outputs, and destroys it afterwards.

`task test-tf TEST=catalog_discovery`, `TEST=billing_report` and
`TEST=legacy` run only read-only tests. The pod, CPU pod, volume, template,
endpoint, pod-lifecycle and logs tests create real resources for the duration
of the run — set `RUNPOD_API_BASE_URL` to target stage. Of those, a pod bills
per hour while it runs and the network volume bills for allocated storage; the
endpoints are scaled to zero workers and the template is metadata, so neither
costs anything to keep.

Three tests need more than an API key. `registry` takes a real registry login
and an optional ECR ARN from `TF_VAR_registry_username`,
`TF_VAR_registry_password` and `TF_VAR_ecr_resource_arn`, and fails without
the login. `cluster` creates a real multi-node cluster, and `legacy_machine`
registers a host machine through the deprecated GraphQL API; both need an
account that is allowed to do so. The by-id CPU flavor lookup is left unset in
`catalog_discovery`: flavor identifiers vary by account visibility, so no
default can be asserted.

`task test-e2e` is the whole live suite in one command
(`scripts/test_e2e.sh`). A pre-flight checks the API key, the local build for
the platform terraform runs as, and the terraform version, and prints which
hosts the run will reach: REST v2 at `RUNPOD_API_BASE_URL`, and the deprecated
REST v1 and GraphQL shims at `RUNPOD_REST_V1_URL` and `RUNPOD_GRAPHQL_URL`,
each production when unset. Then it runs every `tests/*.tftest.hcl` file
cheapest first, one `terraform test` per file so a failure does not stop the
rest, skips any file whose prerequisites are missing and says why, and always
ends with a read-only sweep (`scripts/leftovers/main.tf`) that lists everything
on the account still named `tf-test-`, then a pass, fail and skip summary. It
exits non-zero when a test failed.

```bash
E2E_DRY_RUN=1 task test-e2e                 # pre-flight and plan only; runs nothing
task test-e2e                               # every file whose prerequisites are met
E2E_ONLY="template registry" task test-e2e  # just these files
```

`registry` runs when both `TF_VAR_registry_*` values are set. `cluster` bills
two H100 80GB pods and runs only with `E2E_CLUSTER=1`; `legacy_machine` needs
a host account and runs only with `E2E_HOST_ACCOUNT=1`. `account_ssh_keys`
replaces the account's whole SSH key set and leaves its test key behind, so
it runs only with `E2E_SSH_KEYS=yes`. Each run writes
a timestamped folder under `E2E_LOG_ROOT` (default `artifacts/e2e`, which git
ignores) holding `e2e.log`, one log per test file, `leftovers.log` and
`summary.txt`.
