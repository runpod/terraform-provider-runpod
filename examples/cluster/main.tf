terraform {
  required_providers {
    runpod = {
      source = "runpod/runpod"
    }
  }
}

provider "runpod" {}

# Baseline Instant Cluster: 2 full nodes (8 GPUs each) in a private pool's
# data center. Clusters are GraphQL-only on the backend; this resource passes
# `type` through unvalidated, so new cluster types (e.g. RAY) need no provider
# release once enabled server-side.
resource "runpod_cluster" "baseline" {
  name              = "baseline-lifecycle-tf"
  type              = "SLURM"
  gpu_type_id       = "NVIDIA H100 80GB HBM3"
  pod_count         = 2
  gpu_count_per_pod = 8
  data_center_ids   = ["AP-IN-2"]
  deploy_cost       = 80.0

  ports = "22/tcp"

  env = {
    NCCL_DEBUG = "INFO"
  }
}

output "cluster_id" {
  value = runpod_cluster.baseline.id
}

# Pod details (roles, interconnect IPs, status) populate on refresh:
#   terraform refresh && terraform output cluster_pods
output "cluster_pods" {
  value = runpod_cluster.baseline.pods
}
