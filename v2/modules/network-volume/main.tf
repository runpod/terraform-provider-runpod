resource "runpod_network_volume" "this" {
  name        = var.name
  size        = var.size
  data_center = var.data_center
  type        = var.type
}

# Every network volume on the account, including the one above.
data "runpod_network_volumes" "all" {
  depends_on = [runpod_network_volume.this]
}
