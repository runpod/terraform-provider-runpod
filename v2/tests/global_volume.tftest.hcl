provider "runpod" {}

# The GPU the pod run creates. A run block can only read variables declared in
# its own test file, so it is declared here.
variable "gpu_type_id" {
  type    = string
  default = "NVIDIA L40S"
}

# Creates a real global volume, renames it in place and reads it back by id,
# then mounts it on a GPU pod and on an endpoint scaled to zero workers, moves
# the pod's mount path in place and detaches the volume from the endpoint.
# terraform test destroys the pod and the endpoint before the volume.

run "create" {
  module {
    source = "../modules/global-volume"
  }

  variables {
    name = "tf-test-global-volume"
  }

  assert {
    condition     = output.id != ""
    error_message = "global volume id is empty"
  }
  assert {
    condition     = output.name == "tf-test-global-volume"
    error_message = "global volume name does not round-trip"
  }
  assert {
    condition     = output.lookup_name == output.name
    error_message = "the runpod_global_volume data source read back a different name"
  }
}

run "rename" {
  module {
    source = "../modules/global-volume"
  }

  variables {
    name = "tf-test-global-volume-renamed"
  }

  assert {
    condition     = output.name == "tf-test-global-volume-renamed"
    error_message = "in-place rename did not round-trip"
  }
}

run "mount" {
  module {
    source = "./fixtures/global-volume-mounts"
  }

  variables {
    volume_id           = run.rename.id
    gpu_type_id         = var.gpu_type_id
    pod_path            = "/global"
    endpoint_volume_ids = [run.rename.id]
  }

  assert {
    condition     = length(output.pod_global_volumes) == 1 && output.pod_global_volumes[0].volume_id == run.rename.id
    error_message = "the pod does not report the global volume"
  }
  assert {
    condition     = output.pod_global_volumes[0].path == "/global"
    error_message = "the pod's mount path did not round-trip"
  }
  assert {
    condition     = output.endpoint_global_volumes == tolist([run.rename.id])
    error_message = "the endpoint does not report the global volume"
  }
}

run "move_and_detach" {
  module {
    source = "./fixtures/global-volume-mounts"
  }

  variables {
    volume_id           = run.rename.id
    gpu_type_id         = var.gpu_type_id
    pod_path            = "/data"
    endpoint_volume_ids = []
  }

  assert {
    condition     = output.pod_id == run.mount.pod_id
    error_message = "changing the mount path replaced the pod"
  }
  assert {
    condition     = output.pod_global_volumes[0].path == "/data"
    error_message = "the new mount path did not round-trip"
  }
  assert {
    condition     = output.endpoint_id == run.mount.endpoint_id
    error_message = "detaching the global volume replaced the endpoint"
  }
  assert {
    condition     = try(length(output.endpoint_global_volumes), 0) == 0
    error_message = "the endpoint still reports the global volume after []"
  }
}
