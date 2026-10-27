resource "runpod_global_volume" "this" {
  name = var.name
}

# The volume read back by id, as a pod or endpoint configuration that only
# knows the id would see it.
data "runpod_global_volume" "this" {
  id = runpod_global_volume.this.id
}
