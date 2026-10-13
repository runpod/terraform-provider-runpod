data "runpod_network_volumes" "all" {}

output "volume_sizes" {
  description = "Allocated size in GB per volume, keyed by volume id."
  value       = { for v in data.runpod_network_volumes.all.network_volumes : v.id => v.size }
}
