data "runpod_cpu_types" "all" {}

output "cpu_ids" {
  description = "CPU flavor identifiers."
  value       = [for c in data.runpod_cpu_types.all.cpu_types : c.id]
}
