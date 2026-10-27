---
page_title: "Registry credentials and ECR delegation"
subcategory: "Use-case modules"
description: |-
  Stores container registry credentials for pulling private images and, when ecr_resource_arn is given, delegates pulls for an AWS ECR resource.
---

# Registry credentials and ECR delegation

Stores container registry credentials for pulling private images and,
when `ecr_resource_arn` is given, delegates pulls for an AWS ECR resource.
Both are free to keep. The credential `id` is the `registry` input of the
pod, endpoint and template modules; the delegation's `docker_registry_uri`
is the registry images then resolve through.

Both need real external input, passed as variables rather than hardcoded:

```bash
terraform apply -var username=... -var password=... \
  -var ecr_resource_arn=arn:aws:ecr:us-east-1:123456789012:repository/trainers
```

Neither resource supports `terraform import`: the API never returns the
credential secret or the delegated ARN, so an imported resource could not be
reconciled against configuration. The listings read back every credential
and delegation on the account. The `registry_usernames` output exists for
1.x configurations and is always null: no supported API reports a
credential's username.

## Usage

```hcl
module "registry" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/registry?ref=v2.0.0-rc.4"

  name     = "ghcr"
  username = var.registry_username
  password = var.registry_password
}
```

## Resources and data sources

- [`runpod_registry`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/registry)
- [`runpod_registry_delegation`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/registry_delegation)
- [`runpod_registries`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/registries)
- [`runpod_registry_delegations`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/registry_delegations)

## Configuration

```terraform
resource "runpod_registry" "this" {
  name     = var.name
  username = var.username
  password = var.password
}

# An AWS ECR pull delegation, when an ARN is given. The API never echoes the
# ARN back, so the resource is the only place `resource` is visible.
resource "runpod_registry_delegation" "this" {
  count = var.ecr_resource_arn == null ? 0 : 1

  name     = var.delegation_name
  resource = var.ecr_resource_arn
}

# Every credential and every delegation on the account, including the ones
# above. Secrets are never returned.
data "runpod_registries" "all" {
  depends_on = [runpod_registry.this]
}

data "runpod_registry_delegations" "all" {
  depends_on = [runpod_registry_delegation.this]
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0-rc.4/v2/modules/registry/README.md).
