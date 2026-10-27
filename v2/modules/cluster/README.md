# Multi-node cluster

Creates a cluster of identical GPU pods on one private overlay network, then
reads it back through the `runpod_cluster`, `runpod_cluster_pods` and
`runpod_clusters` data sources.

Billed per hour for **every** member pod while the cluster exists:
`pod_count * gpu_count_per_pod` GPUs in total, so the default (two H100 pods)
costs several dollars an hour. Destroy when done.

Only the name can be changed afterwards: the compute shape, cluster type, and
container configuration are fixed at creation, so changing any of them
replaces the cluster and every pod in it. The member pods cannot be managed
with the pod modules: they are excluded from the account-wide pod listing
and are created and destroyed with the cluster. `primary_ssh_endpoint` is how
you drive it; SSH needs keys registered through
[account-ssh-keys](../account-ssh-keys).

Build and run it with the shared flow in [../README.md](../README.md), then
`terraform destroy`.

## Usage

```hcl
module "cluster" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/cluster?ref=v2.0.0-rc.4"

  name              = "training"
  gpu_type_id       = "NVIDIA H100 80GB HBM3"
  gpu_count_per_pod = 8
  pod_count         = 2
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
| [runpod_cluster.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.4/docs/resources/cluster) | resource |
| [runpod_cluster.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.4/docs/data-sources/cluster) | data source |
| [runpod_cluster_pods.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.4/docs/data-sources/cluster_pods) | data source |
| [runpod_clusters.all](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.4/docs/data-sources/clusters) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| args | Arguments passed to the container entrypoint on every member pod. Null keeps the image's own. | `string` | `null` | no |
| data\_center\_ids | Preferred data centers for placement. Null lets the scheduler choose. | `list(string)` | `null` | no |
| disk | Container disk in GB on each pod (ephemeral). | `number` | `50` | no |
| env | Environment variables set on every member pod. | `map(string)` | `{}` | no |
| gpu\_count\_per\_pod | GPUs on each pod. Total GPUs are pod\_count * gpu\_count\_per\_pod. | `number` | `1` | no |
| gpu\_type\_id | GPU type every pod in the cluster runs on (see the catalog-discovery module). | `string` | `"NVIDIA H100 80GB HBM3"` | no |
| image | Docker image every member pod runs. Public images only; a cluster takes no registry credential. | `string` | `"runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"` | no |
| name | Cluster display name. The only attribute that can change without replacing the cluster. | `string` | `"example-cluster"` | no |
| pod\_count | Pods (nodes) in the cluster. | `number` | `2` | no |
| ports | Exposed ports on every member pod. | `list(string)` | <pre>[<br/>  "22/tcp"<br/>]</pre> | no |
| start\_jupyter | Start JupyterLab on every member pod. Null uses the API default. | `bool` | `null` | no |
| start\_ssh | Provision SSH on every member pod using the account's registered keys (see the account-ssh-keys module). | `bool` | `true` | no |
| type | Cluster type: TRAINING, SLURM, RAY, or APPLICATION. | `string` | `"TRAINING"` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| cluster\_ids | Identifiers of every cluster on the account, including this one. |
| data\_center\_id | Data center the cluster was placed in; null until a pod is placed. |
| id | Cluster identifier. |
| lookup\_type | Cluster type as the runpod\_cluster data source reads it back by id. |
| network\_cidr | CIDR of the private overlay network the member pods share. |
| pods | Member pods with their cluster ranks, roles, and overlay addresses. |
| pods\_by\_status | Member pod counts keyed by pod status. |
| pods\_total | Member pods currently provisioned. |
| primary\_ssh\_endpoint | SSH endpoint of the primary node, which is how you drive the cluster. Null until it is RUNNING with SSH exposed. |
<!-- END_TF_DOCS -->
