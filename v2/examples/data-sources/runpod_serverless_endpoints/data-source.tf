data "runpod_serverless_endpoints" "all" {}

output "endpoint_names" {
  description = "Endpoint names, keyed by endpoint id."
  value       = { for e in data.runpod_serverless_endpoints.all.endpoints : e.id => e.name }
}
