# Deprecated GraphQL shim; removed when REST v2 has a machines route.

data "runpod_machines" "all" {}

output "listed_machines" {
  description = "Ids of the machines currently listed for others to rent."
  value       = [for m in data.runpod_machines.all.machines : m.id if m.listed]
}
