output "gpu_types" {
  description = "Full GPU catalog with per-cloud availability and pricing."
  value       = data.runpod_gpu_types.all.gpu_types
}

output "gpu_ids" {
  description = "GPU type identifiers, usable as gpu_type_id in the gpu-pod and cluster modules."
  value       = [for g in data.runpod_gpu_types.all.gpu_types : g.id]
}

output "gpu_pools" {
  description = "Serverless GPU pool per GPU type, keyed by type id, for the serverless-endpoint module's gpu_pools; types outside any pool are omitted."
  value       = { for g in data.runpod_gpu_types.all.gpu_types : g.id => g.pool if g.pool != null }
}

output "gpu_type" {
  description = "The looked-up GPU type: name, pool, memory, prices and cloud availability."
  value       = data.runpod_gpu_type.this
}

output "cpu_types" {
  description = "Full CPU flavor catalog with vCPU bounds and pricing, for the cpu-pod and serverless-endpoint-cpu modules."
  value       = data.runpod_cpu_types.all.cpu_types
}

output "cpu_type" {
  description = "The looked-up CPU flavor; null when cpu_type_id is unset."
  value       = one(data.runpod_cpu_type.this)
}

output "data_centers" {
  description = "All data centers with region, compliance, and capabilities."
  value       = data.runpod_data_centers.all.data_centers
}

output "global_network_ids" {
  description = "Data centers where global networking is available."
  value       = [for dc in data.runpod_data_centers.all.data_centers : dc.id if dc.global_network]
}

output "data_center" {
  description = "The looked-up data center: name, region and capabilities."
  value       = data.runpod_data_center.this
}

output "catalog_templates" {
  description = "Public templates in the selected catalog slice."
  value       = data.runpod_catalog_templates.this.templates
}

output "catalog_template_ids" {
  description = "Catalog template identifiers, usable as template_id in the pod and endpoint modules."
  value       = [for t in data.runpod_catalog_templates.this.templates : t.id]
}

output "serverless_template_images" {
  description = "Images of the catalog templates that configure Serverless workers."
  value       = [for t in data.runpod_catalog_templates.this.templates : t.image if t.serverless]
}
