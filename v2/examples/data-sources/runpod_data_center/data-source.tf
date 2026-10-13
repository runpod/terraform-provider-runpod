data "runpod_data_center" "eu_ro_1" {
  id = "EU-RO-1"
}

output "network_volume_types" {
  description = "Storage tiers the data center offers."
  value       = data.runpod_data_center.eu_ro_1.network_volume_types
}
