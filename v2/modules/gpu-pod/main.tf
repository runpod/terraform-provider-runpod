resource "runpod_pod" "this" {
  name        = var.name
  image       = var.image
  template_id = var.template_id
  args        = var.args

  gpu_type_id            = var.gpu_type_id
  gpu_count              = var.gpu_count
  allowed_cuda_versions  = var.allowed_cuda_versions
  min_cuda_version       = var.min_cuda_version
  min_ram_per_gpu        = var.min_ram_per_gpu
  min_vcpu_count_per_gpu = var.min_vcpu_count_per_gpu

  cloud             = var.cloud
  disk              = var.disk
  data_center_ids   = var.data_center_ids
  global_networking = var.global_networking
  registry          = var.registry
  desired_state     = var.desired_state

  # At most one mount kind. persistent_volume is host-local; network_volumes
  # reference a runpod_network_volume that outlives the pod.
  persistent_volume = var.persistent_volume
  network_volumes   = var.network_volumes

  start_ssh     = var.start_ssh
  start_jupyter = var.start_jupyter

  env   = var.env
  ports = var.ports
}
