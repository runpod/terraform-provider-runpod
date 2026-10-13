data "runpod_network_volume_billing" "storage" {
  bucket_size = "month"
  last_n      = 6
}

output "storage_spend_by_tier" {
  description = "Standard and high-performance storage spend over the window, in USD."
  value = {
    standard         = data.runpod_network_volume_billing.storage.metadata.totals.standard_amount
    high_performance = data.runpod_network_volume_billing.storage.metadata.totals.high_performance_amount
  }
}
