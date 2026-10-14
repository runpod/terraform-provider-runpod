---
page_title: "Template"
subcategory: "Storage and images"
description: |-
  Creates a reusable pod / endpoint configuration template, reads it back by id, and lists every template visible to the account.
---

# Template

Creates a reusable pod / endpoint configuration template, reads it back by id
through the `runpod_template` data source, and lists every template visible
to the account. Configuration metadata only, so it is free to keep.

The template's `id` is what the
[gpu-pod](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/gpu-pod),
[cpu-pod](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/cpu-pod)
and Serverless endpoint modules take as `template_id`; `serverless = true`
marks a template for endpoints. A private image needs a `registry`
credential (see
[registry](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/registry)).
`persistent_volume` gives every container created from the template a
host-local mount.

The deprecated inputs and outputs (`docker_entrypoint`, `readme`, `earned`,
`is_runpod`, `runtime_in_min`) go through the REST v1 compatibility shim and
exist for 1.x configurations.

## Usage

```hcl
module "template" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/template?ref=v2.0.0"

  name  = "pytorch-base"
  image = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  disk  = 20
}
```

## Resources and data sources

- [`runpod_template`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/template)
- [`runpod_template`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/template) (data source)
- [`runpod_templates`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/templates)

## Configuration

```terraform
resource "runpod_template" "this" {
  name       = var.name
  image      = var.image
  args       = var.args
  disk       = var.disk
  env        = var.env
  ports      = var.ports
  serverless = var.serverless
  public     = var.public
  category   = var.category

  registry              = var.registry
  start_ssh             = var.start_ssh
  start_jupyter         = var.start_jupyter
  allowed_cuda_versions = var.allowed_cuda_versions

  # Host-local persistent storage for containers created from the template,
  # replacing the 1.x volume_in_gb and volume_mount_path pair.
  persistent_volume = var.persistent_volume

  # Deprecated, served by the REST v1 shim: the 1.x docker_entrypoint and
  # readme attributes. Both are applied on create and update, and both are read
  # back, so a template that already has them fills them in.
  docker_entrypoint = var.docker_entrypoint
  readme            = var.readme
}

# The template read back by id — what a pod or endpoint pointing at it sees.
data "runpod_template" "this" {
  id = runpod_template.this.id
}

# Every template visible to the account, including the one above.
data "runpod_templates" "all" {
  depends_on = [runpod_template.this]
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0/v2/modules/template/README.md).
