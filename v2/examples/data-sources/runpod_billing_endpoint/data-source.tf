# Deprecated: use runpod_serverless_billing, which reads REST v2. This one stays
# for time_billed_ms, which REST v2 billing does not carry yet.

data "runpod_billing_endpoint" "one_endpoint" {
  bucket_size = "hour"
  endpoint_id = "endpoint_abc123"
}

output "endpoint_time_billed_ms" {
  description = "How long the endpoint's workers billed for, in milliseconds."
  value = sum(concat([0], [
    for r in data.runpod_billing_endpoint.one_endpoint.billing_records :
    r.time_billed_ms if r.time_billed_ms != null
  ]))
}
