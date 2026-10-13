output "id" {
  description = "Pod identifier."
  value       = runpod_pod.this.id
}

output "status" {
  description = "Pod lifecycle status as reported by the API."
  value       = runpod_pod.this.status
}

output "data_center_id" {
  description = "Data center the scheduler placed the pod in."
  value       = runpod_pod.this.data_center_id
}

output "image" {
  description = "Image the pod runs, whether set directly or resolved from a template."
  value       = runpod_pod.this.image
}

output "cpu_type_id" {
  description = "CPU flavor the pod runs on."
  value       = runpod_pod.this.cpu_type_id
}

output "vcpu_count" {
  description = "vCPUs allocated to the pod."
  value       = runpod_pod.this.vcpu_count
}

output "gpu_type_id" {
  description = "GPU type of the pod; always null for a CPU pod."
  value       = runpod_pod.this.gpu_type_id
}

output "network_volumes" {
  description = "Network volumes mounted into the pod, if any."
  value       = runpod_pod.this.network_volumes
}

output "cost" {
  description = "Current cost in USD per hour, zero while the pod is stopped."
  value       = runpod_pod.this.cost
}

output "locked" {
  description = "Whether the pod is locked against stopping or resetting."
  value       = runpod_pod.this.locked
}

output "memory_in_gb" {
  description = "System RAM in GB allocated to the pod."
  value       = runpod_pod.this.memory_in_gb
}

output "started_at" {
  description = "When the pod last started, in RFC 3339 format."
  value       = runpod_pod.this.started_at
}

output "runtime_uptime_seconds" {
  description = "Seconds since the pod's container started; null unless the pod is RUNNING."
  value       = try(runpod_pod.this.runtime.uptime_seconds, null)
}

# Deprecated read-backs served by the REST v1 shim, preserving the 1.x
# machine_id and volume_encrypted attributes. Null when the shim host does not
# answer.
output "machine_id" {
  description = "Deprecated. Host machine the pod is placed on."
  value       = runpod_pod.this.machine_id
}

output "volume_encrypted" {
  description = "Deprecated. Whether the pod's volume is encrypted."
  value       = runpod_pod.this.volume_encrypted
}
