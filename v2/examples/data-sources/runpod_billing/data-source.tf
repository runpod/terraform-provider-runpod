data "runpod_billing" "last_week" {
  bucket_size = "day"
  last_n      = 7
}

output "weekly_spend" {
  description = "Total spend over the last seven days, in USD."
  value       = data.runpod_billing.last_week.metadata.totals.total_amount
}

output "daily_spend" {
  description = "Total spend per day, keyed by the bucket's start."
  value       = { for r in data.runpod_billing.last_week.records : r.start_time => r.amounts.total_amount }
}
