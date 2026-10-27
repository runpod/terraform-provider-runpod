---
page_title: "Global volume"
subcategory: "Use-case modules"
description: |-
  Creates a global volume, usage-billed storage with no data center, and reads it back by id.
---

# Global volume

Creates a global volume and reads it back by id. A global volume is
usage-billed storage with no data center: a
[`runpod_pod`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/pod)
mounts it through its `global_volumes` attribute wherever the pod is placed,
and a GPU
[`runpod_serverless_endpoint`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/serverless_endpoint)
mounts it on every worker through its own `global_volumes`. Only the name can
change.

## Usage

```hcl
module "global_volume" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/global-volume?ref=v2.0.0"

  name = "datasets"
}
```

## Resources and data sources

- [`runpod_global_volume`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/global_volume)
- [`runpod_global_volume`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/global_volume) (data source)

## Configuration

```terraform
resource "runpod_global_volume" "this" {
  name = var.name
}

# The volume read back by id, as a pod or endpoint configuration that only
# knows the id would see it.
data "runpod_global_volume" "this" {
  id = runpod_global_volume.this.id
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0/v2/modules/global-volume/README.md).
