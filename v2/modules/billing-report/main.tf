# Time-bucketed spend over the last last_n buckets: the account-wide total
# plus one data source per billed resource kind. Every read is free.
data "runpod_billing" "this" {
  bucket_size = var.bucket_size
  last_n      = var.last_n
}

data "runpod_pod_billing" "this" {
  bucket_size = var.bucket_size
  last_n      = var.last_n
  pod_id      = var.pod_id
}

data "runpod_serverless_billing" "this" {
  bucket_size   = var.bucket_size
  last_n        = var.last_n
  serverless_id = var.serverless_id
}

data "runpod_endpoint_billing" "this" {
  bucket_size = var.bucket_size
  last_n      = var.last_n
}

data "runpod_network_volume_billing" "this" {
  bucket_size       = var.bucket_size
  last_n            = var.last_n
  network_volume_id = var.network_volume_id
}

data "runpod_cluster_billing" "this" {
  bucket_size = var.bucket_size
  last_n      = var.last_n
  cluster_id  = var.cluster_id
}
