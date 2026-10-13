data "runpod_serverless_billing" "all_endpoints" {
  bucket_size = "month"
  last_n      = 3
}

output "serverless_spend_by_endpoint" {
  description = "Spend per endpoint per bucket, in USD."
  value = {
    for r in data.runpod_serverless_billing.all_endpoints.records :
    "${r.serverless_id}/${r.start_time}" => r.amounts.total_amount
  }
}
