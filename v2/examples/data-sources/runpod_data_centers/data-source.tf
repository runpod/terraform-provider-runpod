data "runpod_data_centers" "all" {}

output "global_network_ids" {
  description = "Data centers where global networking is available."
  value       = [for dc in data.runpod_data_centers.all.data_centers : dc.id if dc.global_network]
}
