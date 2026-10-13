data "runpod_clusters" "all" {}

output "training_cluster_gpu_totals" {
  description = "Total GPUs per TRAINING cluster in the account."
  value = {
    for c in data.runpod_clusters.all.clusters : c.name => c.pod_count * c.gpu_count_per_pod
    if c.type == "TRAINING"
  }
}
