output "id" {
  description = "Volume identifier, the volume_id the pod modules and the network_volumes list of the endpoint modules take."
  value       = runpod_network_volume.this.id
}

output "name" {
  description = "Volume name."
  value       = runpod_network_volume.this.name
}

output "size" {
  description = "Allocated storage in GB."
  value       = runpod_network_volume.this.size
}

output "data_center" {
  description = "Data center the volume lives in; pass it as data_center_ids to whatever mounts it."
  value       = runpod_network_volume.this.data_center
}

output "type" {
  description = "Effective storage tier (API default when not configured)."
  value       = runpod_network_volume.this.type
}

output "network_volume_ids" {
  description = "Identifiers of every network volume on the account, including this one."
  value       = [for v in data.runpod_network_volumes.all.network_volumes : v.id]
}
