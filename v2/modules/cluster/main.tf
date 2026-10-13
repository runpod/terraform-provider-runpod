resource "runpod_cluster" "this" {
  name              = var.name
  type              = var.type
  gpu_type_id       = var.gpu_type_id
  gpu_count_per_pod = var.gpu_count_per_pod
  pod_count         = var.pod_count
  data_center_ids   = var.data_center_ids

  image         = var.image
  args          = var.args
  disk          = var.disk
  env           = var.env
  ports         = var.ports
  start_ssh     = var.start_ssh
  start_jupyter = var.start_jupyter
}

# The cluster read back through the data sources: the cluster object itself,
# its member pods with their ranks and overlay addresses, and every cluster on
# the account.
data "runpod_cluster" "this" {
  id = runpod_cluster.this.id
}

data "runpod_cluster_pods" "this" {
  cluster_id = runpod_cluster.this.id
}

data "runpod_clusters" "all" {
  depends_on = [runpod_cluster.this]
}
