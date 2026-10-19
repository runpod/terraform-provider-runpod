---
page_title: "Network volume"
subcategory: "Use-case modules"
description: |-
  Creates a persistent network storage volume and lists every volume on the account.
---

# Network volume

Creates a persistent network storage volume and lists every volume on the
account. Billed for the allocated storage while it exists.

The volume is bound to its data center: a pod from
[gpu-pod](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/gpu-pod)
or
[cpu-pod](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/cpu-pod)
mounts it through `network_volumes` and has to be placed in the same data
center (`data_center_ids`); the Serverless endpoint modules take the `id` in
their `network_volumes` list. Size can grow in place but never shrink.

## Usage

```hcl
module "network_volume" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/network-volume?ref=v2.0.0-rc.3"

  name        = "datasets"
  size        = 100
  data_center = "EU-RO-1"
}
```

## Resources and data sources

- [`runpod_network_volume`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/network_volume)
- [`runpod_network_volumes`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/network_volumes)

## Configuration

```terraform
resource "runpod_network_volume" "this" {
  name        = var.name
  size        = var.size
  data_center = var.data_center
  type        = var.type
}

# Every network volume on the account, including the one above.
data "runpod_network_volumes" "all" {
  depends_on = [runpod_network_volume.this]
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0-rc.3/v2/modules/network-volume/README.md).
