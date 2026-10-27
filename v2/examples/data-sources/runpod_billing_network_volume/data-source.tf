# Deprecated: use runpod_network_volume_billing, which reads the same REST v2
# route with the cost split by storage tier. This one keeps the 1.x arguments
# and record shape so a 1.x configuration still plans; disk_space_billed_gb and
# time_billed_ms always read null.

data "runpod_billing_network_volume" "one_volume" {
  bucket_size       = "day"
  network_volume_id = "netvol_abc123"
}

output "volume_spend" {
  description = "What the volume cost over the window, in USD."
  value = sum(concat([0], [
    for r in data.runpod_billing_network_volume.one_volume.billing_records :
    r.amount if r.amount != null
  ]))
}
