# Deprecated: use runpod_network_volume_billing, which reads REST v2. This one
# stays for disk_space_billed_gb and time_billed_ms, which REST v2 billing does
# not carry yet.

data "runpod_billing_network_volume" "one_volume" {
  bucket_size       = "day"
  network_volume_id = "netvol_abc123"
}

output "volume_disk_gb" {
  description = "Disk space billed across the window, in GB."
  value = sum(concat([0], [
    for r in data.runpod_billing_network_volume.one_volume.billing_records :
    r.disk_space_billed_gb if r.disk_space_billed_gb != null
  ]))
}
