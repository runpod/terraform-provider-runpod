output "id" {
  description = "Volume identifier, the volume_id a pod's global_volumes and the id an endpoint's global_volumes take."
  value       = runpod_global_volume.this.id
}

output "name" {
  description = "Volume name."
  value       = runpod_global_volume.this.name
}

output "created_at" {
  description = "When the volume was created, in RFC 3339 format."
  value       = runpod_global_volume.this.created_at
}

output "lookup_name" {
  description = "The name the runpod_global_volume data source reads back by id."
  value       = data.runpod_global_volume.this.name
}
