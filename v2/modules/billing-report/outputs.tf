output "window" {
  description = "The window the API resolved the request to."
  value       = data.runpod_billing.this.metadata.query
}

output "total_spend" {
  description = "Total spend over the window, in USD."
  value       = data.runpod_billing.this.metadata.totals.total_amount
}

output "spend_by_bucket" {
  description = "Total spend per time bucket, keyed by the bucket's start."
  value       = { for r in data.runpod_billing.this.records : r.start_time => r.amounts.total_amount }
}

output "pod_spend" {
  description = "Pod spend over the window, in USD."
  value       = data.runpod_pod_billing.this.metadata.totals.total_amount
}

output "pod_count" {
  description = "Number of distinct pods the records span."
  value       = data.runpod_pod_billing.this.metadata.unique_pod_count
}

output "serverless_spend" {
  description = "Serverless spend over the window, in USD."
  value       = data.runpod_serverless_billing.this.metadata.totals.total_amount
}

output "endpoint_spend" {
  description = "Spend on Runpod's public endpoints over the window, in USD."
  value       = data.runpod_endpoint_billing.this.metadata.totals.total_amount
}

output "network_volume_spend" {
  description = "Network volume storage spend over the window, in USD."
  value       = data.runpod_network_volume_billing.this.metadata.totals.total_amount
}

output "cluster_spend" {
  description = "Instant Cluster spend over the window, in USD."
  value       = data.runpod_cluster_billing.this.metadata.totals.total_amount
}
