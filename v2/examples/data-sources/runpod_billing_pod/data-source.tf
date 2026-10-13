# Deprecated: use runpod_pod_billing, which reads REST v2. This one stays for
# the per-record fields REST v2 billing does not carry yet: gpu_type_id,
# disk_space_billed_gb, endpoint_id and time_billed_ms.

data "runpod_billing_pod" "a100" {
  bucket_size = "day"
  gpu_type_id = "NVIDIA A100 80GB PCIe"
}

output "a100_spend" {
  description = "What A100 pods cost over the window, in USD."
  value       = sum(concat([0], [for r in data.runpod_billing_pod.a100.billing_records : r.amount if r.amount != null]))
}

output "a100_time_billed_ms" {
  description = "How long those pods billed for, in milliseconds."
  value       = sum(concat([0], [for r in data.runpod_billing_pod.a100.billing_records : r.time_billed_ms if r.time_billed_ms != null]))
}
