data "runpod_cluster" "training" {
  id = "cluster_abc123"
}

output "cluster_ready" {
  description = "Whether every member pod of the cluster is running."
  value       = lookup(data.runpod_cluster.training.pods_by_status, "RUNNING", 0) == data.runpod_cluster.training.pods_total
}

output "entry_point" {
  description = "SSH endpoint of the primary node; null until it is RUNNING with SSH exposed."
  value       = data.runpod_cluster.training.primary_ssh_endpoint
}
