# GPU pod

Runs one GPU pod, billed per hour until you destroy it. The
default is a secure-cloud RTX 4090 (`cloud = "SECURE"`), which is more
reliable to schedule and bills at secure-cloud rates; set
`cloud = "COMMUNITY"` for the cheaper tier. For a CPU-only pod use the
[cpu-pod](../cpu-pod) module instead.

Set `template_id` instead of `image` to take the container settings from a
pod template (see [template](../template)). Storage comes in two forms, at
most one per pod: `persistent_volume` (`{ size, path }`, host-local, replaces
the 1.x `volume_in_gb` and `volume_mount_path`) and `network_volumes`
(`{ volume_id, path }` entries pointing at a volume from the
[network-volume](../network-volume) module, replacing the 1.x
`network_volume_id`). A network volume pins the pod to the volume's data
center, so pass the same value in `data_center_ids`.

`desired_state` turns the pod's run state into managed configuration:
`RUNNING` restarts a pod stopped in the console on the next apply, `EXITED`
keeps it stopped. For one-off start / stop / restart calls on an existing pod
use [pod-lifecycle](../pod-lifecycle).

The two deprecated outputs (`machine_id`, `volume_encrypted`) come from the
REST v1 compatibility shim and read null when that host does not answer.

Build and run it with the shared flow in [../README.md](../README.md), then
`terraform destroy`.

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| terraform | >= 1.6 |
| runpod | ~> 2.0 |

## Providers

| Name | Version |
| ---- | ------- |
| runpod | ~> 2.0 |

## Resources

| Name | Type |
| ---- | ---- |
| [runpod_pod.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/pod) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| allowed\_cuda\_versions | Exact set of acceptable host CUDA versions, e.g. ["12.8"]. Null accepts any; a non-empty set cannot be combined with min\_cuda\_version. | `list(string)` | `null` | no |
| args | Arguments passed to the container entrypoint. Null keeps the image's own. | `string` | `null` | no |
| cloud | Cloud tier: SECURE or COMMUNITY. COMMUNITY is cheaper but capacity for a given GPU type fluctuates. | `string` | `"SECURE"` | no |
| data\_center\_ids | Preferred data centers for placement. Null lets the scheduler choose; a network volume requires the pod to land in the volume's data center. | `list(string)` | `null` | no |
| desired\_state | Managed run state: RUNNING or EXITED. When set, every apply starts or stops the pod to match, and a pod stopped outside Terraform shows up as a plan diff. Null leaves the run state unmanaged. | `string` | `null` | no |
| disk | Container disk in GB (ephemeral). | `number` | `20` | no |
| env | Environment variables. | `map(string)` | `{}` | no |
| global\_networking | Enable global networking (a private cross-datacenter IP). Needs an eligible data center. Null leaves it off. | `bool` | `null` | no |
| gpu\_count | Number of GPUs. | `number` | `1` | no |
| gpu\_type\_id | GPU type from the catalog (see the catalog-discovery module). | `string` | `"NVIDIA GeForce RTX 4090"` | no |
| image | Docker image reference. Null when template\_id supplies one. | `string` | `"runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"` | no |
| min\_cuda\_version | Lowest acceptable host CUDA version, e.g. "12.1". Null sets no floor. | `string` | `null` | no |
| min\_ram\_per\_gpu | Minimum host system RAM in GB per requested GPU. A placement filter: raising it reduces the machines that can take the pod. | `number` | `null` | no |
| min\_vcpu\_count\_per\_gpu | Minimum host vCPU count per requested GPU. A placement filter: raising it reduces the machines that can take the pod. | `number` | `null` | no |
| name | Pod display name. | `string` | `"example-gpu-pod"` | no |
| network\_volumes | Network volumes to mount (see the network-volume module), replacing the 1.x network\_volume\_id attribute. Each volume must live in the same data center as the pod. Null attaches none, and the API takes one entry. | <pre>list(object({<br/>    volume_id = string<br/>    path      = string<br/>  }))</pre> | `null` | no |
| persistent\_volume | Host-local persistent storage, replacing the 1.x volume\_in\_gb and volume\_mount\_path attributes. Null attaches none. Mutually exclusive with network\_volumes. | <pre>object({<br/>    size = number<br/>    path = string<br/>  })</pre> | `null` | no |
| ports | Exposed ports. | `list(string)` | <pre>[<br/>  "8888/http",<br/>  "22/tcp"<br/>]</pre> | no |
| registry | Registry credential ID (see the registry module) for pulling a private image. Null for public images. | `string` | `null` | no |
| start\_jupyter | Start JupyterLab at create time, reachable on the "8888/http" port. | `bool` | `true` | no |
| start\_ssh | Set up SSH access at create time. Needs an SSH key registered on the account (see the account-ssh-keys module), and "22/tcp" in ports for the direct form. | `bool` | `true` | no |
| template\_id | Pod template to base the pod on (see the template module). Null builds the pod from image alone. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| cluster\_id | Cluster the pod belongs to; null for a standalone pod. |
| cost | Current cost in USD per hour, zero while the pod is stopped. |
| cpu\_type\_id | CPU flavor of the pod; always null for a GPU pod. |
| cuda\_version | CUDA version the host machine reports. |
| data\_center\_id | Data center the scheduler placed the pod in. |
| docker\_entrypoint | Container entrypoint the pod runs, in exec form, as the API decodes it from args. |
| docker\_start\_cmd | Container start command the pod runs, in exec form, as the API decodes it from args. |
| gpu\_type\_id | GPU type the pod runs on. |
| id | Pod identifier. |
| image | Image the pod runs, whether set directly or resolved from a template. |
| locked | Whether the pod is locked against stopping or resetting. |
| machine\_id | Deprecated. Host machine the pod is placed on. |
| memory\_in\_gb | System RAM in GB allocated to the pod. |
| network\_volumes | Network volumes mounted into the pod, if any. |
| persistent\_volume | Host-local persistent mount attached to the pod, if any. |
| runtime | Live utilization while the pod runs: uptime\_seconds, cpu\_util, memory\_util, per-GPU utilization, and the port mappings it was assigned. Null unless the pod is RUNNING. |
| runtime\_uptime\_seconds | Seconds since the pod's container started; null unless the pod is RUNNING. |
| started\_at | When the pod last started, in RFC 3339 format. |
| status | Pod lifecycle status as reported by the API. |
| volume\_encrypted | Deprecated. Whether the pod's volume is encrypted. |
<!-- END_TF_DOCS -->
