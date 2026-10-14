---
page_title: "Multi-node cluster"
subcategory: "Compute"
description: |-
  Creates a cluster of identical GPU pods on one private overlay network, then reads it back through the cluster data sources.
---

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
[account-ssh-keys](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/account-ssh-keys).

## Usage

```hcl
module "cluster" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/cluster?ref=v2.0.0"

  name              = "training"
  gpu_type_id       = "NVIDIA H100 80GB HBM3"
  gpu_count_per_pod = 8
  pod_count         = 2
}
```

## Resources and data sources

- [`runpod_cluster`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/cluster)
- [`runpod_cluster`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/cluster) (data source)
- [`runpod_cluster_pods`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/cluster_pods)
- [`runpod_clusters`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/clusters)

## Configuration

```terraform
resource "runpod_cluster" "this" {
  name              = var.name
  type              = var.type
  gpu_type_id       = var.gpu_type_id
  gpu_count_per_pod = var.gpu_count_per_pod
  pod_count         = var.pod_count
  data_center_ids   = var.data_center_ids

  image         = var.image
  args          = var.args
  disk          = var.disk
  env           = var.env
  ports         = var.ports
  start_ssh     = var.start_ssh
  start_jupyter = var.start_jupyter
}

# The cluster read back through the data sources: the cluster object itself,
# its member pods with their ranks and overlay addresses, and every cluster on
# the account.
data "runpod_cluster" "this" {
  id = runpod_cluster.this.id
}

data "runpod_cluster_pods" "this" {
  cluster_id = runpod_cluster.this.id
}

data "runpod_clusters" "all" {
  depends_on = [runpod_cluster.this]
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0/v2/modules/cluster/README.md).
