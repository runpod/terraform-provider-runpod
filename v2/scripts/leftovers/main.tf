# Read-only sweep for what the acceptance tests can leave behind: everything on
# the account whose name starts with the tf-test- prefix the test files use.
# scripts/test_e2e.sh applies it after every live run. It declares only data
# sources, so an apply reads, creates nothing and costs nothing. An interrupted
# run, or a destroy that errors, is what leaves objects here.
#
# Not covered: the account_ssh_keys test key, which that test leaves on purpose
# (remove_on_destroy = false) and which carries no tf-test- name.

terraform {
  required_version = ">= 1.6"

  required_providers {
    runpod = {
      source  = "runpod/runpod"
      version = "2.0.0-rc.3"
    }
  }
}

provider "runpod" {}

variable "include_machines" {
  description = "Also list host machines. They are read through the deprecated GraphQL API, and only the legacy_machine test, on a host account, creates one."
  type        = bool
  default     = false
}

locals {
  prefix = "tf-test-"
}

data "runpod_pods" "all" {
  include_cluster_pods = true
}

data "runpod_network_volumes" "all" {}
data "runpod_templates" "all" {}
data "runpod_serverless_endpoints" "all" {}
data "runpod_clusters" "all" {}
data "runpod_registries" "all" {}
data "runpod_registry_delegations" "all" {}

data "runpod_machines" "all" {
  count = var.include_machines ? 1 : 0
}

# try() turns a null name, which a delegation created without one has, into a
# non-match instead of an error.
locals {
  leftovers = {
    pods        = { for p in data.runpod_pods.all.pods : p.id => p.name if try(startswith(p.name, local.prefix), false) }
    volumes     = { for v in data.runpod_network_volumes.all.network_volumes : v.id => v.name if try(startswith(v.name, local.prefix), false) }
    templates   = { for t in data.runpod_templates.all.templates : t.id => t.name if try(startswith(t.name, local.prefix), false) }
    endpoints   = { for e in data.runpod_serverless_endpoints.all.endpoints : e.id => e.name if try(startswith(e.name, local.prefix), false) }
    clusters    = { for c in data.runpod_clusters.all.clusters : c.id => c.name if try(startswith(c.name, local.prefix), false) }
    registries  = { for r in data.runpod_registries.all.registries : r.id => r.name if try(startswith(r.name, local.prefix), false) }
    delegations = { for d in data.runpod_registry_delegations.all.delegations : d.id => d.name if try(startswith(d.name, local.prefix), false) }
  }

  machines = { for m in flatten(data.runpod_machines.all[*].machines) : m.id => m.name if try(startswith(m.name, local.prefix), false) }
}

output "leftovers" {
  description = "Everything on the account still named with the acceptance-test prefix, as id => name per type."
  value       = local.leftovers
}

output "machines" {
  description = "Host machines named with the prefix; null, and so not shown, unless include_machines is set."
  value       = var.include_machines ? local.machines : null
}

output "leftover_count" {
  description = "How many objects the two outputs above list; 0 on a clean account."
  value       = sum([for group in local.leftovers : length(group)]) + length(local.machines)
}
