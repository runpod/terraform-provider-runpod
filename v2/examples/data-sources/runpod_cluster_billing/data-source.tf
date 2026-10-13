data "runpod_cluster_billing" "one_cluster" {
  cluster_id  = "cluster_abc123"
  bucket_size = "day"
  last_n      = 14
}

output "cluster_spend" {
  description = "What the cluster cost over the last 14 days, in USD."
  value       = data.runpod_cluster_billing.one_cluster.metadata.totals.total_amount
}

output "cluster_networking_spend" {
  description = "The inter-node networking share of that cost, in USD."
  value       = data.runpod_cluster_billing.one_cluster.metadata.totals.networking_amount
}
