data "runpod_cluster_pods" "training" {
  cluster_id = "cluster_abc123"
}

output "overlay_addresses_by_rank" {
  description = "Each member pod's address on the cluster's private network, keyed by node rank."
  value = {
    for p in data.runpod_cluster_pods.training.pods : p.cluster_rank => p.cluster_ip
    if p.cluster_rank != null
  }
}
