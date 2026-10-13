data "runpod_pod_billing" "one_pod" {
  pod_id      = "pod_abc123"
  bucket_size = "day"
  last_n      = 30
}

output "pod_spend" {
  description = "What the pod cost over the last 30 days, in USD."
  value       = data.runpod_pod_billing.one_pod.metadata.totals.total_amount
}

output "pod_disk_spend" {
  description = "The disk share of that cost, in USD."
  value       = data.runpod_pod_billing.one_pod.metadata.totals.disk_amount
}
