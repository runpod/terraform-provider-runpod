# One state transition on a pod that already exists. The resource is
# imperative: the action runs on create and again whenever action or triggers
# change; a refresh calls nothing and destroying it only forgets it.
resource "runpod_pod_action" "this" {
  pod_id   = var.pod_id
  action   = var.action
  triggers = var.triggers
}

# The pod after the transition, and every pod on the account.
data "runpod_pod" "this" {
  id = var.pod_id

  depends_on = [runpod_pod_action.this]
}

data "runpod_pods" "all" {
  include_cluster_pods = var.include_cluster_pods

  depends_on = [runpod_pod_action.this]
}
