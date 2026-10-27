data "runpod_global_volume" "training_data" {
  id = "gsb_abc"
}

output "global_volume_name" {
  description = "Name of the global volume."
  value       = data.runpod_global_volume.training_data.name
}

output "global_volume_created_at" {
  description = "When the global volume was created, in RFC 3339 format."
  value       = data.runpod_global_volume.training_data.created_at
}
