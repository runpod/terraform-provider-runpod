output "id" {
  description = "The pod the action ran against."
  value       = runpod_pod_action.this.id
}

output "action" {
  description = "The transition that ran."
  value       = runpod_pod_action.this.action
}

output "status" {
  description = "Deprecated. Pod status as the action's GraphQL shim reported it right after the transition; null when that host does not answer."
  value       = runpod_pod_action.this.status
}

output "pod_status" {
  description = "Pod lifecycle status read back after the transition."
  value       = data.runpod_pod.this.status
}

output "pod_cost_per_hour" {
  description = "What the pod costs per hour now, in USD; zero once stopped."
  value       = data.runpod_pod.this.cost
}

output "pod_gpu_type_id" {
  description = "GPU type the pod was placed on; null for a CPU pod."
  value       = data.runpod_pod.this.gpu_type_id
}

output "pod_ssh_command" {
  description = "Proxy SSH command for the pod; null until it has a machine assignment."
  value       = try(data.runpod_pod.this.ssh.proxy.command, null)
}

output "pod_uptime_seconds" {
  description = "Seconds since the pod's container started; null unless the pod is RUNNING."
  value       = try(data.runpod_pod.this.runtime.uptime_seconds, null)
}

output "pod_ids" {
  description = "Identifiers of every pod on the account."
  value       = [for p in data.runpod_pods.all.pods : p.id]
}
