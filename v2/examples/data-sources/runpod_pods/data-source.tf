data "runpod_pods" "all" {}

output "running_pod_ids" {
  description = "Pods currently reported as running."
  value       = [for p in data.runpod_pods.all.pods : p.id if p.status == "RUNNING"]
}
