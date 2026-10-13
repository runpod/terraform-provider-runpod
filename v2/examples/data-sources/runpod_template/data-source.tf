data "runpod_template" "pytorch" {
  id = "tpl_abc"
}

output "image" {
  description = "Image the template deploys."
  value       = data.runpod_template.pytorch.image
}

output "allowed_cuda_versions" {
  description = "CUDA versions the template accepts; empty means any."
  value       = data.runpod_template.pytorch.allowed_cuda_versions
}
