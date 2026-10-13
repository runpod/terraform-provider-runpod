resource "runpod_cluster" "training" {
  name              = "llama-finetune"
  type              = "TRAINING"
  gpu_type_id       = "NVIDIA H100 80GB HBM3"
  gpu_count_per_pod = 8
  pod_count         = 4
  image             = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  disk              = 100
  ports             = ["22/tcp"]
  start_ssh         = true

  env = {
    NCCL_DEBUG = "INFO"
  }

  # A network volume shared by every member pod, in the same data center as the
  # cluster. Use mounts.persistent instead for host-local scratch space.
  mounts = {
    network = [{
      volume_id = runpod_network_volume.datasets.id
      path      = "/runpod-volume"
    }]
  }
}

output "cluster_entry_point" {
  description = "SSH into the primary node to drive the cluster."
  value       = runpod_cluster.training.primary_ssh_endpoint
}
