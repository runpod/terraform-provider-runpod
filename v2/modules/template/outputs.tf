output "id" {
  description = "Template identifier, usable as template_id in the pod and endpoint modules."
  value       = runpod_template.this.id
}

output "image" {
  description = "Image the template deploys."
  value       = runpod_template.this.image
}

output "serverless" {
  description = "Whether the template is serverless-compatible (API-derived)."
  value       = runpod_template.this.serverless
}

output "start_ssh" {
  description = "Whether containers from this template provision SSH access (API default when not configured)."
  value       = runpod_template.this.start_ssh
}

output "start_jupyter" {
  description = "Whether containers from this template start JupyterLab (API default when not configured)."
  value       = runpod_template.this.start_jupyter
}

output "allowed_cuda_versions" {
  description = "Effective CUDA version constraint; empty means any version."
  value       = runpod_template.this.allowed_cuda_versions
}

output "persistent_volume" {
  description = "Persistent mount containers from this template receive, if any."
  value       = runpod_template.this.persistent_volume
}

output "lookup_image" {
  description = "Image as the runpod_template data source reads it back by id."
  value       = data.runpod_template.this.image
}

output "lookup_start_ssh" {
  description = "SSH flag as the runpod_template data source reads it back by id."
  value       = data.runpod_template.this.start_ssh
}

output "template_ids" {
  description = "Identifiers of every template visible to the account, including this one."
  value       = [for t in data.runpod_templates.all.templates : t.id]
}

# Deprecated attributes served by the REST v1 shim, preserving the 1.x
# docker_entrypoint, readme, earned, is_runpod and runtime_in_min attributes.
output "docker_entrypoint" {
  description = "Deprecated. Container entrypoint containers from this template run."
  value       = runpod_template.this.docker_entrypoint
}

output "readme" {
  description = "Deprecated. Markdown description shown on the template's console page."
  value       = runpod_template.this.readme
}

output "earned" {
  description = "Deprecated. Earnings in USD credited to this template through the creator programme; zero for a private template."
  value       = runpod_template.this.earned
}

output "is_runpod" {
  description = "Deprecated. Whether Runpod itself publishes the template."
  value       = runpod_template.this.is_runpod
}

output "runtime_in_min" {
  description = "Deprecated. Total minutes containers created from this template have run."
  value       = runpod_template.this.runtime_in_min
}

output "lookup_docker_entrypoint" {
  description = "Container entrypoint as the runpod_template data source reads it back from the v2 template."
  value       = data.runpod_template.this.docker_entrypoint
}
