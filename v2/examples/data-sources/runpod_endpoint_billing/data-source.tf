data "runpod_endpoint_billing" "public_endpoints" {
  start_time  = "2026-06-01T00:00:00Z"
  end_time    = "2026-07-01T00:00:00Z"
  bucket_size = "day"
}

output "public_endpoint_spend" {
  description = "Spend on Runpod's public endpoints in June 2026, in USD."
  value       = data.runpod_endpoint_billing.public_endpoints.metadata.totals.total_amount
}
