---
page_title: "GPU pod"
subcategory: "Compute"
description: |-
  Runs one GPU pod.
---

# GPU pod

Runs one GPU pod, billed per hour until you destroy it. The
default is a secure-cloud RTX 4090 (`cloud = "SECURE"`), which is more
reliable to schedule and bills at secure-cloud rates; set
`cloud = "COMMUNITY"` for the cheaper tier. For a CPU-only pod use the
[cpu-pod](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/cpu-pod)
module instead.

Set `template_id` instead of `image` to take the container settings from a
pod template (see
[template](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/template)).
Storage comes in two forms, at most one per pod: `persistent_volume`
(`{ size, path }`, host-local, replaces the 1.x `volume_in_gb` and
`volume_mount_path`) and `network_volumes` (`{ volume_id, path }` entries
pointing at a volume from the
[network-volume](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/network-volume)
module, replacing the 1.x `network_volume_id`). A network volume pins the pod
to the volume's data center, so pass the same value in `data_center_ids`.

`desired_state` turns the pod's run state into managed configuration:
`RUNNING` restarts a pod stopped in the console on the next apply, `EXITED`
keeps it stopped. For one-off start / stop / restart calls on an existing pod
use
[pod-lifecycle](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/pod-lifecycle).

The two deprecated outputs (`machine_id`, `volume_encrypted`) come from the
REST v1 compatibility shim and read null when that host does not answer.

## Usage

```hcl
module "gpu_pod" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/gpu-pod?ref=v2.0.0"

  name        = "training"
  gpu_type_id = "NVIDIA GeForce RTX 4090"
  disk        = 50
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

  gpu_type_id            = var.gpu_type_id
  gpu_count              = var.gpu_count
  allowed_cuda_versions  = var.allowed_cuda_versions
  min_cuda_version       = var.min_cuda_version
  min_ram_per_gpu        = var.min_ram_per_gpu
  min_vcpu_count_per_gpu = var.min_vcpu_count_per_gpu

  cloud             = var.cloud
  disk              = var.disk
  data_center_ids   = var.data_center_ids
  global_networking = var.global_networking
  registry          = var.registry
  desired_state     = var.desired_state

  # At most one mount kind. persistent_volume is host-local; network_volumes
  # reference a runpod_network_volume that outlives the pod.
  persistent_volume = var.persistent_volume
  network_volumes   = var.network_volumes

  start_ssh     = var.start_ssh
  start_jupyter = var.start_jupyter

  env   = var.env
  ports = var.ports
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0/v2/modules/gpu-pod/README.md).
