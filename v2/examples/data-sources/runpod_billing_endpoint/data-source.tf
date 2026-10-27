# Deprecated: use runpod_serverless_billing, which reads the same REST v2 route
# with the cost components broken out. This one keeps the 1.x arguments and
# record shape so a 1.x configuration still plans; time_billed_ms comes from
# the GraphQL API's serverless billing, at whole-second precision.

data "runpod_billing_endpoint" "one_endpoint" {
  bucket_size = "hour"
  endpoint_id = "endpoint_abc123"
}

output "endpoint_spend" {
  description = "What the endpoint's workers cost over the window, in USD."
  value = sum(concat([0], [
    for r in data.runpod_billing_endpoint.one_endpoint.billing_records :
    r.amount if r.amount != null
  ]))
}

output "endpoint_time_billed_ms" {
  description = "How long the endpoint's workers billed for, in milliseconds."
  value = sum(concat([0], [
    for r in data.runpod_billing_endpoint.one_endpoint.billing_records :
    r.time_billed_ms if r.time_billed_ms != null
  ]))
}
