resource "runpod_template" "pytorch" {
  name  = "pytorch-notebook"
  image = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  disk  = 20

  # Containers created from this template get SSH and JupyterLab through the
  # Runpod image conventions. Both default to true when omitted.
  start_ssh     = true
  start_jupyter = true

  # Host-local persistent storage for every container created from the
  # template, replacing the 1.x volume_in_gb and volume_mount_path pair.
  persistent_volume = {
    size = 20
    path = "/workspace"
  }

  # Only place containers on hosts reporting one of these CUDA versions.
  allowed_cuda_versions = ["12.8", "12.6"]

  env = {
    MODEL_NAME = "llama-3"
  }

  ports = ["8888/http", "22/tcp"]
}

# What the deprecated GraphQL shim reports back about a template's usage,
# preserving the 1.x earned, is_runpod and runtime_in_min attributes.
output "pytorch_runtime_in_min" {
  value = runpod_template.pytorch.runtime_in_min
}

output "pytorch_is_runpod" {
  value = runpod_template.pytorch.is_runpod
}

# A serverless worker template pulling from a private registry. registry takes
# a runpod_registry credential id, called container_registry_auth_id in 1.x.
resource "runpod_template" "worker" {
  name          = "sdxl-worker"
  image         = "my-org/sdxl-worker:latest"
  category      = "NVIDIA"
  serverless    = true
  disk          = 30
  registry      = runpod_registry.private.id
  start_ssh     = false
  start_jupyter = false
}

resource "runpod_registry" "private" {
  name     = "my-org-registry"
  username = "my-org"
  password = var.registry_password
}

variable "registry_password" {
  description = "Registry password or access token."
  type        = string
  sensitive   = true
}
