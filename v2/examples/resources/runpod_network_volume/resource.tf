resource "runpod_network_volume" "datasets" {
  name        = "datasets"
  size        = 100
  data_center = "EU-RO-1"
  type        = "HIGH_PERFORMANCE"
}
