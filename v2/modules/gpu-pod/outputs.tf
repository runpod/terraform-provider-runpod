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

output "gpu_type_id" {
  description = "GPU type the pod runs on."
  value       = runpod_pod.this.gpu_type_id
}

output "cpu_type_id" {
  description = "CPU flavor of the pod; always null for a GPU pod."
  value       = runpod_pod.this.cpu_type_id
}

output "persistent_volume" {
  description = "Host-local persistent mount attached to the pod, if any."
  value       = runpod_pod.this.persistent_volume
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

output "cuda_version" {
  description = "CUDA version the host machine reports."
  value       = runpod_pod.this.cuda_version
}

output "memory_in_gb" {
  description = "System RAM in GB allocated to the pod."
  value       = runpod_pod.this.memory_in_gb
}

output "started_at" {
  description = "When the pod last started, in RFC 3339 format."
  value       = runpod_pod.this.started_at
}

output "cluster_id" {
  description = "Cluster the pod belongs to; null for a standalone pod."
  value       = runpod_pod.this.cluster_id
}

output "runtime" {
  description = "Live utilization while the pod runs: uptime_seconds, cpu_util, memory_util, per-GPU utilization, and the port mappings it was assigned. Null unless the pod is RUNNING."
  value       = runpod_pod.this.runtime
}

output "runtime_uptime_seconds" {
  description = "Seconds since the pod's container started; null unless the pod is RUNNING."
  value       = try(runpod_pod.this.runtime.uptime_seconds, null)
}

output "docker_entrypoint" {
  description = "Container entrypoint the pod runs, in exec form, as the API decodes it from args."
  value       = runpod_pod.this.docker_entrypoint
}

output "docker_start_cmd" {
  description = "Container start command the pod runs, in exec form, as the API decodes it from args."
  value       = runpod_pod.this.docker_start_cmd
}

# Deprecated read-backs served by the GraphQL shim, preserving the 1.x
# machine_id and volume_encrypted attributes. Null when that API does not
# answer or does not report the pod.
output "machine_id" {
  description = "Deprecated. Host machine the pod is placed on."
  value       = runpod_pod.this.machine_id
}

output "volume_encrypted" {
  description = "Deprecated. Whether the pod's volume is encrypted."
  value       = runpod_pod.this.volume_encrypted
}
