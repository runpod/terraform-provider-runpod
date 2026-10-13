data "runpod_gpu_types" "all" {}

output "gpu_ids" {
  description = "GPU type identifiers, usable as runpod_pod.gpu_type_id."
  value       = [for g in data.runpod_gpu_types.all.gpu_types : g.id]
}
