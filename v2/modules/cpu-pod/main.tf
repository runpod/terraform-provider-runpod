resource "runpod_pod" "this" {
  name        = var.name
  image       = var.image
  template_id = var.template_id
  args        = var.args

  # A CPU pod: cpu_type_id and vcpu_count instead of gpu_type_id.
  cpu_type_id = var.cpu_type_id
  vcpu_count  = var.vcpu_count

  cloud           = var.cloud
  disk            = var.disk
  data_center_ids = var.data_center_ids
  registry        = var.registry
  desired_state   = var.desired_state

  # CPU pods take no host-local persistent volume; a network volume is the
  # storage that outlives the pod.
  network_volumes = var.network_volumes

  start_ssh     = var.start_ssh
  start_jupyter = var.start_jupyter

  env   = var.env
  ports = var.ports
}
