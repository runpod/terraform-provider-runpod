data "runpod_cpu_type" "cpu3c" {
  id = "cpu3c"
}

output "vcpu_max" {
  description = "Largest vCPU count the flavor can be provisioned with."
  value       = data.runpod_cpu_type.cpu3c.vcpu_max
}
