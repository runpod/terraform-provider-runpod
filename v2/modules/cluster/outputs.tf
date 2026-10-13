output "id" {
  description = "Cluster identifier."
  value       = runpod_cluster.this.id
}

output "data_center_id" {
  description = "Data center the cluster was placed in; null until a pod is placed."
  value       = runpod_cluster.this.data_center_id
}

output "pods_total" {
  description = "Member pods currently provisioned."
  value       = runpod_cluster.this.pods_total
}

output "pods_by_status" {
  description = "Member pod counts keyed by pod status."
  value       = runpod_cluster.this.pods_by_status
}

output "primary_ssh_endpoint" {
  description = "SSH endpoint of the primary node, which is how you drive the cluster. Null until it is RUNNING with SSH exposed."
  value       = runpod_cluster.this.primary_ssh_endpoint
}

output "network_cidr" {
  description = "CIDR of the private overlay network the member pods share."
  value       = runpod_cluster.this.network_cidr
}

output "lookup_type" {
  description = "Cluster type as the runpod_cluster data source reads it back by id."
  value       = data.runpod_cluster.this.type
}

output "pods" {
  description = "Member pods with their cluster ranks, roles, and overlay addresses."
  value       = data.runpod_cluster_pods.this.pods
}

output "cluster_ids" {
  description = "Identifiers of every cluster on the account, including this one."
  value       = [for c in data.runpod_clusters.all.clusters : c.id]
}
