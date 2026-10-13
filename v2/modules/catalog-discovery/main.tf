# The catalogs a configuration picks its identifiers from: GPU types and
# Serverless pools, CPU flavors, data centers, and the publicly shared
# templates. Every read is free and safe against any account.
data "runpod_gpu_types" "all" {}

data "runpod_gpu_type" "this" {
  id = var.gpu_type_id
}

data "runpod_cpu_types" "all" {}

# Flavor identifiers vary by account visibility, so the by-id CPU lookup runs
# only when one is given.
data "runpod_cpu_type" "this" {
  count = var.cpu_type_id == null ? 0 : 1

  id = var.cpu_type_id
}

data "runpod_data_centers" "all" {}

data "runpod_data_center" "this" {
  id = var.data_center_id
}

data "runpod_catalog_templates" "this" {
  source = var.catalog_slice
}
