data "runpod_registries" "all" {}

output "registry_ids" {
  description = "Credential identifiers, referenced from private-image pods and endpoints."
  value       = { for r in data.runpod_registries.all.registries : r.name => r.id }
}
