# Deprecated GraphQL shim; removed when REST v2 has a machines route.

data "runpod_machine" "rig" {
  id = "m4h2k9qzx1"
}

output "gpu_headroom" {
  description = "GPUs on the machine that are not reserved."
  value       = data.runpod_machine.rig.gpu_total - data.runpod_machine.rig.gpu_reserved
}
