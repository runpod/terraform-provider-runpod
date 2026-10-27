# Deprecated: use runpod_pod_billing, which reads the REST v2 route with the
# cost components broken out. This one keeps the 1.x arguments and record
# shape so a 1.x configuration still plans.

# One pod, from REST v2: gpu_type_id, endpoint_id, disk_space_billed_gb and
# time_billed_ms read null.
data "runpod_billing_pod" "one_pod" {
  bucket_size = "day"
  pod_id      = "pod_abc123"
}

# One GPU type, the way REST v1 grouped pod billing, from the GraphQL billing
# API: gpu_type_id, amount, time and time_billed_ms are filled.
data "runpod_billing_pod" "a100" {
  bucket_size = "day"
  gpu_type_id = "NVIDIA A100 80GB PCIe"
}

output "pod_spend" {
  description = "What the pod cost over the window, in USD."
  value       = sum(concat([0], [for r in data.runpod_billing_pod.one_pod.billing_records : r.amount if r.amount != null]))
}

output "a100_time_billed_ms" {
  description = "How long A100 pods billed for over the window, in milliseconds."
  value       = sum(concat([0], [for r in data.runpod_billing_pod.a100.billing_records : r.time_billed_ms if r.time_billed_ms != null]))
}
