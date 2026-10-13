data "runpod_catalog_templates" "official" {
  source = "official"
}

output "catalog_template_ids" {
  description = "Identifiers of the Runpod-curated templates."
  value       = [for t in data.runpod_catalog_templates.official.templates : t.id]
}
