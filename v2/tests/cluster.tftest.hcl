provider "runpod" {}

# Creates a real, multi-node, billed cluster: pod_count * gpu_count_per_pod
# GPUs for as long as the run takes. Kept to the smallest shape the API
# accepts, two pods with one GPU each, and terraform test destroys it afterwards.
# Needs an account that is allowed to create clusters.

run "create" {
  module {
    source = "../modules/cluster"
  }

  variables {
    name              = "tf-test-cluster"
    type              = "TRAINING"
    gpu_count_per_pod = 1
    pod_count         = 2
    disk              = 50
  }

  assert {
    condition     = output.id != ""
    error_message = "cluster id is empty"
  }
  assert {
    condition     = output.lookup_type == "TRAINING"
    error_message = "the runpod_cluster data source did not read back the cluster type"
  }
  assert {
    condition     = output.pods_total == 2
    error_message = "cluster did not provision the requested two member pods"
  }
  assert {
    condition     = length(output.pods) == 2
    error_message = "the runpod_cluster_pods data source did not list both member pods"
  }
  assert {
    condition     = alltrue([for p in output.pods : p.id != ""])
    error_message = "cluster pods listing returned an entry with an empty id"
  }
  assert {
    condition     = output.network_cidr != ""
    error_message = "cluster overlay network has no CIDR"
  }
  assert {
    condition     = contains(output.cluster_ids, output.id)
    error_message = "the new cluster is missing from the clusters listing"
  }
  assert {
    condition     = alltrue([for id in output.cluster_ids : id != ""])
    error_message = "clusters listing returned an entry with an empty id"
  }
}
