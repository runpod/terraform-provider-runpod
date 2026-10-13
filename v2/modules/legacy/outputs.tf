output "machine_id" {
  description = "Deprecated. Identifier of the registered machine; null unless create_machine is set."
  value       = try(one(runpod_machine.this).id, null)
}

output "machine_name" {
  description = "Deprecated. Name of the registered machine; null unless create_machine is set."
  value       = try(one(runpod_machine.this).name, null)
}

output "machine_gpu_total" {
  description = "Deprecated. GPUs the API reports for the registered machine, read back through the data source; null unless create_machine is set."
  value       = try(one(data.runpod_machine.this).gpu_total, null)
}

output "machine_names" {
  description = "Deprecated. Name per machine on the account, keyed by machine id. Null unless list_machines or create_machine is set."
  value       = length(data.runpod_machines.all) > 0 ? { for m in data.runpod_machines.all[0].machines : m.id => m.name } : null
}

output "account_id" {
  description = "Deprecated. Identifier of the account the API key belongs to."
  value       = data.runpod_user.this.id
}

output "account_email" {
  description = "Deprecated. Email of the account the API key belongs to."
  value       = data.runpod_user.this.email
}

output "pod_record_count" {
  description = "Deprecated. Number of 1.x pod billing records in the window."
  value       = length(data.runpod_billing_pod.this.billing_records)
}

output "pod_spend" {
  description = "Deprecated. Pod spend over the window, in USD."
  value       = sum(concat([0], [for r in data.runpod_billing_pod.this.billing_records : r.amount if r.amount != null]))
}

output "pod_time_billed_ms" {
  description = "Deprecated. Pod time billed over the window, in milliseconds. REST v2 billing does not report it."
  value       = sum(concat([0], [for r in data.runpod_billing_pod.this.billing_records : r.time_billed_ms if r.time_billed_ms != null]))
}

output "pod_gpu_types" {
  description = "Deprecated. GPU types the pod records bill, deduplicated. REST v2 billing does not report them."
  value       = distinct([for r in data.runpod_billing_pod.this.billing_records : r.gpu_type_id if r.gpu_type_id != null])
}

output "network_volume_record_count" {
  description = "Deprecated. Number of 1.x network volume billing records in the window."
  value       = length(data.runpod_billing_network_volume.this.billing_records)
}

output "network_volume_disk_gb" {
  description = "Deprecated. Disk space billed across the network volume records, in GB. REST v2 billing does not report it."
  value       = sum(concat([0], [for r in data.runpod_billing_network_volume.this.billing_records : r.disk_space_billed_gb if r.disk_space_billed_gb != null]))
}

output "endpoint_record_count" {
  description = "Deprecated. Number of 1.x Serverless endpoint billing records in the window."
  value       = length(data.runpod_billing_endpoint.this.billing_records)
}

output "endpoint_spend" {
  description = "Deprecated. Serverless endpoint spend over the window, in USD."
  value       = sum(concat([0], [for r in data.runpod_billing_endpoint.this.billing_records : r.amount if r.amount != null]))
}
