---
page_title: "CPU pod"
subcategory: "Use-case modules"
description: |-
  Runs one CPU-only pod: a CPU flavor (cpu_type_id) and a vCPU count instead of a GPU type.
---

# CPU pod

Runs one CPU-only pod: a CPU flavor (`cpu_type_id`) and a vCPU count instead
of a GPU type. Billed per hour while it runs, at CPU rates. Flavor identifiers and their valid vCPU counts come from the CPU catalog
(see
[catalog-discovery](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/catalog-discovery));
the count has to be a power of two inside the flavor's range.

A CPU pod takes no host-local persistent volume. Storage that outlives the
pod is a network volume from the
[network-volume](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/network-volume)
module, mounted through `network_volumes` and pinned to the volume's data
center through `data_center_ids`.

The two deprecated outputs (`machine_id`, `volume_encrypted`) come from the
REST v1 compatibility shim and read null when that host does not answer.

## Usage

```hcl
module "cpu_pod" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/cpu-pod?ref=v2.0.0-rc.3"

  name        = "preprocessing"
  cpu_type_id = "cpu5c"
  vcpu_count  = 4
}
```

## Resources and data sources

- [`runpod_pod`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/pod)

## Configuration

```terraform
resource "runpod_pod" "this" {
  name        = var.name
  image       = var.image
  template_id = var.template_id
  args        = var.args

  # A CPU pod: cpu_type_id and vcpu_count instead of gpu_type_id.
  cpu_type_id = var.cpu_type_id
  vcpu_count  = var.vcpu_count

  cloud           = var.cloud
  disk            = var.disk
  data_center_ids = var.data_center_ids
  registry        = var.registry
  desired_state   = var.desired_state

  # CPU pods take no host-local persistent volume; a network volume is the
  # storage that outlives the pod.
  network_volumes = var.network_volumes

  start_ssh     = var.start_ssh
  start_jupyter = var.start_jupyter

  env   = var.env
  ports = var.ports
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0-rc.3/v2/modules/cpu-pod/README.md).
