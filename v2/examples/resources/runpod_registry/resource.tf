variable "registry_password" {
  description = "Registry password or token."
  type        = string
  sensitive   = true
}

resource "runpod_registry" "private" {
  name     = "private-images"
  username = "runpod-puller"
  password = var.registry_password
}

# Reference the credential id from any resource that pulls a private image.
resource "runpod_pod" "private" {
  name        = "private-image-pod"
  image       = "ghcr.io/example/trainer:1.0.0"
  gpu_type_id = "NVIDIA GeForce RTX 4090"
  registry    = runpod_registry.private.id
}
