data "runpod_gpu_type" "rtx4090" {
  id = "NVIDIA GeForce RTX 4090"
}

output "vram_gb" {
  description = "VRAM of the looked-up GPU type, in GB."
  value       = data.runpod_gpu_type.rtx4090.memory
}
