# CPU pod

Runs one CPU-only pod: a CPU flavor (`cpu_type_id`) and a vCPU count instead
of a GPU type. Billed per hour while it runs, at CPU rates. Flavor identifiers and their valid vCPU counts come from the CPU catalog
(see [catalog-discovery](../catalog-discovery)); the count has to be a power
of two inside the flavor's range.

A CPU pod takes no host-local persistent volume. Storage that outlives the
pod is a network volume from the [network-volume](../network-volume) module,
mounted through `network_volumes` and pinned to the volume's data center
through `data_center_ids`.

The two deprecated outputs (`machine_id`, `volume_encrypted`) come from the
GraphQL compatibility shim and read null when that API does not answer or
does not report the pod. `volume_encrypted` reads `false`, not null, for an
unencrypted volume.

Build and run it with the shared flow in [../README.md](../README.md), then
`terraform destroy`.

## Usage

```hcl
module "cpu_pod" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/cpu-pod?ref=v2.0.0-rc.4"

  name        = "preprocessing"
  cpu_type_id = "cpu5c"
  vcpu_count  = 4
}
```

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| terraform | >= 1.6 |
| runpod | 2.0.0-rc.4 |

## Providers

| Name | Version |
| ---- | ------- |
| runpod | 2.0.0-rc.4 |

## Resources

| Name | Type |
| ---- | ---- |
| [runpod_pod.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.4/docs/resources/pod) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| args | Arguments passed to the container entrypoint. Null keeps the image's own. | `string` | `null` | no |
| cloud | Cloud tier: SECURE or COMMUNITY. | `string` | `"SECURE"` | no |
| cpu\_type\_id | CPU flavor from the catalog (see the catalog-discovery module). | `string` | `"cpu5c"` | no |
| data\_center\_ids | Preferred data centers for placement. Null lets the scheduler choose; a network volume requires the pod to land in the volume's data center. | `list(string)` | `null` | no |
| desired\_state | Managed run state: RUNNING or EXITED. When set, every apply starts or stops the pod to match, and a pod stopped outside Terraform shows up as a plan diff. Null leaves the run state unmanaged. | `string` | `null` | no |
| disk | Container disk in GB (ephemeral). | `number` | `20` | no |
| env | Environment variables. | `map(string)` | `{}` | no |
| image | Docker image reference. Null when template\_id supplies one. | `string` | `"runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"` | no |
| name | Pod display name. | `string` | `"example-cpu-pod"` | no |
| network\_volumes | Network volumes to mount (see the network-volume module), replacing the 1.x network\_volume\_id attribute. Each volume must live in the same data center as the pod. Null attaches none, and the API takes one entry. | <pre>list(object({<br/>    volume_id = string<br/>    path      = string<br/>  }))</pre> | `null` | no |
| ports | Exposed ports. | `list(string)` | <pre>[<br/>  "8888/http",<br/>  "22/tcp"<br/>]</pre> | no |
| registry | Registry credential ID (see the registry module) for pulling a private image. Null for public images. | `string` | `null` | no |
| start\_jupyter | Start JupyterLab at create time, reachable on the "8888/http" port. | `bool` | `true` | no |
| start\_ssh | Set up SSH access at create time. Needs an SSH key registered on the account (see the account-ssh-keys module), and "22/tcp" in ports for the direct form. | `bool` | `true` | no |
| template\_id | Pod template to base the pod on (see the template module). Null builds the pod from image alone. | `string` | `null` | no |
| vcpu\_count | Number of vCPUs. Must be valid for the flavor and a power of two. | `number` | `4` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| cost | Current cost in USD per hour, zero while the pod is stopped. |
| cpu\_type\_id | CPU flavor the pod runs on. |
| data\_center\_id | Data center the scheduler placed the pod in. |
| gpu\_type\_id | GPU type of the pod; always null for a CPU pod. |
| id | Pod identifier. |
| image | Image the pod runs, whether set directly or resolved from a template. |
| locked | Whether the pod is locked against stopping or resetting. |
| machine\_id | Deprecated. Host machine the pod is placed on. |
| memory\_in\_gb | System RAM in GB allocated to the pod. |
| network\_volumes | Network volumes mounted into the pod, if any. |
| runtime\_uptime\_seconds | Seconds since the pod's container started; null unless the pod is RUNNING. |
| started\_at | When the pod last started, in RFC 3339 format. |
| status | Pod lifecycle status as reported by the API. |
| vcpu\_count | vCPUs allocated to the pod. |
| volume\_encrypted | Deprecated. Whether the pod's volume is encrypted. |
<!-- END_TF_DOCS -->
