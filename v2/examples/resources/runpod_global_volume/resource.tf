resource "runpod_global_volume" "training_data" {
  name = "training-data"
}

# A global volume has no data center, so the pod that mounts it can be placed
# anywhere. The path is optional; the API chooses one when it is left out.
resource "runpod_pod" "trainer" {
  name        = "trainer"
  image       = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  gpu_type_id = "NVIDIA GeForce RTX 4090"

  global_volumes = [{
    volume_id = runpod_global_volume.training_data.id
    path      = "/data"
  }]
}

output "global_volume_id" {
  description = "Identifier of the global volume."
  value       = runpod_global_volume.training_data.id
}
