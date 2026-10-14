---
page_title: "Using the provider from an AI agent"
subcategory: "Getting started"
description: |-
  How an AI agent sets up Runpod, authenticates the provider without a user present, pins versions, reviews plans and cleans up what it creates.
---

# Using the provider from an AI agent

This page is for an AI agent that writes and applies Terraform for Runpod, and
for the person who sets one up.

## If you are an AI agent reading this page

Follow these steps in order. The sections below explain each one.

1. Set up through https://agents.runpod.io first. It installs the Runpod skills
   and connects the Runpod MCP server. Use Terraform for infrastructure that
   persists, and the MCP server for one-off operations.
2. Require the provider as `source = "runpod/runpod"`, `version = "~> 2.0"`.
   Use an exact pin only for a release candidate (see Versions).
3. Read the API key from `RUNPOD_API_KEY` in the environment. Never write a key
   into a `.tf` file, a `.tfvars` file or a command line, and never print it.
   If there is no key, stop and ask the user for one.
4. Start from the use-case module that matches the task. Look up GPU type ids
   and data centers with catalog discovery, not from memory.
5. Run `terraform plan -out=tfplan` and read every resource it replaces or
   destroys. If it replaces or destroys something the task did not ask to
   change, stop and ask.
6. Ask the user before you apply anything that bills: pods, clusters, network
   volumes, Serverless endpoints with workers. Then run `terraform apply tfplan`.
7. Prefix the names of what you create with `agent-`. When the task is done,
   destroy it and confirm with the list data sources that nothing is left.
8. Report provider bugs and documentation errors as
   [GitHub issues](https://github.com/runpod/terraform-provider-runpod/issues),
   and Runpod MCP tool problems through the MCP server's `report_feedback` tool
   when it is listed. Leave keys and secrets out of every report.

## Set up the agent

Point the agent to https://agents.runpod.io. Those instructions
install the Runpod skills from the
[runpod-plugins-official](https://github.com/runpod/runpod-plugins-official)
plugin and connect the hosted Runpod MCP server. Their "No user in the loop"
section covers an agent that runs unattended: it authenticates with an API key
from `RUNPOD_API_KEY` instead of an OAuth sign-in.

## Terraform or the MCP server

Use Terraform for infrastructure that must persist and be reviewed as a plan
before it changes: pods, Serverless endpoints, templates, network volumes,
clusters and registry credentials. Use the MCP server for interactive, one-off
operations: looking something up, checking on a pod, a quick test.

Manage each object one way. Terraform plans against what the API reports, so a
change made to a Terraform-managed object through the MCP server shows up in
the next plan as a change to undo.

## Authentication

The provider uses the `api_key` argument when it is set, and otherwise the
`RUNPOD_API_KEY` environment variable. With neither, it stops before any API
call with "Unable to configure the Runpod client".

For an agent without a user present:

- Set `RUNPOD_API_KEY` in the environment Terraform runs in and leave
  `api_key` out of the configuration.
- Never write the key as a literal in a `.tf` file, a `.tfvars` file or a
  command line, and never print it. If `api_key` has to be set, pass it from a
  sensitive variable:

```hcl
variable "runpod_api_key" {
  type      = string
  sensitive = true
}

provider "runpod" {
  api_key = var.runpod_api_key
}
```

Create a key in the Runpod console; see
[Manage credentials](https://docs.runpod.io/get-started/credentials).

## Versions

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

`~> 2.0` takes every 2.x release. A release candidate, such as `2.0.0-rc.2`,
installs only through an exact pin, `version = "2.0.0-rc.2"`. The use-case
modules require `~> 2.0`, so they do not install next to an exact
release-candidate pin.

## Start from a module

Each use-case guide in this documentation describes a complete, runnable
[use-case module](https://github.com/runpod/terraform-provider-runpod/tree/main/v2/modules).
Start from the one that matches the task instead of writing resources from
scratch:

```hcl
module "gpu_pod" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/gpu-pod?ref=v2.0.0"

  name        = "agent-training"
  gpu_type_id = "NVIDIA GeForce RTX 4090"
  disk        = 50
}
```

Pin `ref` to the release you use. Each module's README lists its inputs and
outputs. Find GPU type ids, CPU flavors and data centers with the
[catalog-discovery](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/catalog-discovery)
module, not from memory.

## Read the plan before applying

Run `terraform plan -out=tfplan`, read it, then `terraform apply tfplan`. Read
every resource the plan replaces (`-/+`, with a `# forces replacement` marker on
the attribute responsible) or destroys before applying. A replaced pod is a new
pod with a new id, and everything not on a network volume is lost. If a plan
replaces or destroys something the task did not ask to change, stop and ask.

Pods need extra care after `terraform import`. The API does not return eight
create-time settings: `template_id`, `start_ssh`, `start_jupyter`,
`allowed_cuda_versions`, `min_cuda_version`, `min_ram_per_gpu`,
`min_vcpu_count_per_gpu` and `data_center_ids`. Import leaves them empty in
state, so each one the configuration sets plans a replacement of the pod. List
them in `lifecycle { ignore_changes = [...] }` before the first apply, as the
"State migration" section of
[Migrating from 1.x to 2.0.0](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/migrating-from-1-x)
shows. Import also reads the pod's whole `env` into state, so the first apply
that sets `env` removes every key the configuration leaves out; the plan lists
those removals.

## Import limits

`runpod_registry`, `runpod_registry_delegation` and `runpod_pod_action` cannot
be imported: the API never returns the registry password or the delegated ARN,
and a pod action holds no state to import. Declaring a registry credential or
delegation again creates a new one. Declaring a pod action again, for example
after it was removed from state, runs the action again.

## Naming and cleanup

- Give test resources a recognisable prefix, such as `agent-`, so they are easy to
  find and to tell apart from anything else on the account.
- Destroy what you create when the task is done: pods and clusters bill while
  they run, a stopped pod still bills for its disk, and network volumes bill
  for their allocated storage.
- Confirm with the list data sources, which list the account's objects:

```hcl
data "runpod_pods" "all" {
  include_cluster_pods = true
}
data "runpod_network_volumes" "all" {}
data "runpod_serverless_endpoints" "all" {}

output "leftovers" {
  value = concat(
    [for p in data.runpod_pods.all.pods : p.id if try(startswith(p.name, "agent-"), false)],
    [for v in data.runpod_network_volumes.all.network_volumes : v.id if try(startswith(v.name, "agent-"), false)],
    [for e in data.runpod_serverless_endpoints.all.endpoints : e.id if try(startswith(e.name, "agent-"), false)],
  )
}
```

`runpod_templates`, `runpod_clusters`, `runpod_registries` and
`runpod_registry_delegations` list the rest.

## Errors

API errors read "API returned status <code>: <response body>" after the
operation that failed, for example "Creating pod failed".

- 401: the API key is missing or wrong. Check `RUNPOD_API_KEY`, or the
  variable that feeds `api_key`.
- 400 with a body saying there are no instances available, or "Insufficient
  resources": a capacity shortage, not a configuration error. Retry with another GPU type, another
  data center, or the other cloud (`cloud = "COMMUNITY"` or `"SECURE"`).

## Reporting problems

Report provider bugs and documentation errors as
[GitHub issues](https://github.com/runpod/terraform-provider-runpod/issues).
For a problem with a Runpod MCP server tool, use the hosted server's
`report_feedback` tool when it is listed; leave keys, secrets and private
project details out of the report.
