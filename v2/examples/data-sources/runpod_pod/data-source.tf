data "runpod_pod" "training" {
  id = "pod_abc123"
}

output "hourly_cost" {
  description = "What the pod costs per hour right now, in USD."
  value       = data.runpod_pod.training.cost
}

output "ssh_command" {
  description = "Proxy SSH command; null until the pod has a machine assignment."
  value       = try(data.runpod_pod.training.ssh.proxy.command, null)
}
