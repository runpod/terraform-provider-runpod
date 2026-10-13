resource "runpod_pod" "training" {
  name        = "training-pod"
  image       = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  gpu_type_id = "NVIDIA GeForce RTX 4090"
  gpu_count   = 1
  cloud       = "SECURE"
  disk        = 20

  # Start SSH and JupyterLab through the Runpod image conventions. SSH needs a
  # key registered on the account; both need their port exposed below.
  start_ssh     = true
  start_jupyter = true

  env = {
    MODEL_NAME = "llama-3"
  }

  ports = ["8888/http", "22/tcp"]
}

# A CPU pod: cpu_type_id replaces gpu_type_id, and vcpu_count comes from the
# flavor's catalog range (a power of two).
resource "runpod_pod" "preprocess" {
  name        = "preprocess-pod"
  image       = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  cpu_type_id = "cpu5c"
  vcpu_count  = 4
  disk        = 20
}

# A pod built from a template instead of an image, with CUDA and host-size
# constraints on placement and a network volume for data that has to outlive
# the pod. persistent_volume { size, path } is the other mount kind; it
# replaces 1.x volume_in_gb / volume_mount_path, and network_volumes replaces
# network_volume_id.
resource "runpod_pod" "from_template" {
  name                   = "template-pod"
  template_id            = "30zmvf89kd"
  gpu_type_id            = "NVIDIA GeForce RTX 4090"
  min_cuda_version       = "12.1"
  min_ram_per_gpu        = 8
  min_vcpu_count_per_gpu = 2

  network_volumes = [{
    volume_id = runpod_network_volume.data.id
    path      = "/runpod-volume"
  }]
}

resource "runpod_network_volume" "data" {
  name        = "training-data"
  size        = 50
  data_center = "EU-RO-1"
}

# What the API reports back about a running pod.
output "training_cost_per_hour" {
  value = runpod_pod.training.cost
}

output "training_memory_in_gb" {
  value = runpod_pod.training.memory_in_gb
}

output "training_cuda_version" {
  value = runpod_pod.training.cuda_version
}

# The command the pod runs, as the exec-form lists the API decodes from args.
output "training_docker_entrypoint" {
  value = runpod_pod.training.docker_entrypoint
}

output "training_docker_start_cmd" {
  value = runpod_pod.training.docker_start_cmd
}

# Deprecated read-backs, preserving the 1.x machine_id and volume_encrypted
# attributes. They come from the REST v1 shim and go away once REST v2 reports
# the same fields; a pod applies fine without the shim, and then they read
# null.
output "training_machine_id" {
  value = runpod_pod.training.machine_id
}

output "training_volume_encrypted" {
  value = runpod_pod.training.volume_encrypted
}
