provider "runpod" {}

# The GPU the pod tests create. A run block can only read variables declared in
# its own test file, so it is declared here. A file-level variables block would
# outrank TF_VAR_gpu_type_id; a variable block's default does not.
variable "gpu_type_id" {
  type    = string
  default = "NVIDIA L40S"
}

# Creates a real (billed) pod; terraform test destroys it afterwards.
# SECURE cloud: community capacity for a given GPU type fluctuates and the
# scheduler 400s when it can't place ("this machine does not have the
# resources"), which makes the test flaky. Secure placement is consistent;
# a few minutes of RTX 4090 costs ~2 cents.

run "create" {
  module {
    source = "../modules/gpu-pod"
  }

  variables {
    gpu_type_id = var.gpu_type_id
    name        = "tf-test-pod"
    cloud       = "SECURE"
    disk        = 20
  }

  assert {
    condition     = output.id != ""
    error_message = "pod id is empty"
  }
  assert {
    condition     = output.status != ""
    error_message = "pod status is empty"
  }
  assert {
    condition     = output.gpu_type_id == var.gpu_type_id
    error_message = "gpu_type_id did not round-trip"
  }
  assert {
    condition     = output.cpu_type_id == null
    error_message = "a GPU pod must report no cpu config"
  }
  assert {
    condition     = output.persistent_volume == null && output.network_volumes == null
    error_message = "a pod with no mount configured must report none"
  }
  assert {
    condition     = output.cost > 0
    error_message = "a running pod must report an hourly cost"
  }
  assert {
    condition     = output.memory_in_gb > 0
    error_message = "the pod's allocated system RAM was not read back"
  }
  assert {
    condition     = output.locked == false
    error_message = "a fresh pod must not be locked"
  }
  assert {
    condition     = output.started_at != null
    error_message = "a running pod must report when it started"
  }
  assert {
    condition     = output.cluster_id == null
    error_message = "a standalone pod must report no cluster"
  }
  # The deprecated GraphQL shim. Both fields are non-null on the GraphQL pod,
  # so a pod placed on a machine reports both; volume_encrypted is false, not
  # null, for an unencrypted volume.
  assert {
    condition     = output.machine_id != null
    error_message = "the GraphQL shim did not report the pod's host machine"
  }
  assert {
    condition     = output.volume_encrypted != null
    error_message = "the GraphQL shim did not report whether the pod's volume is encrypted"
  }
}

# A network volume mounted into the pod, replacing the 1.x network_volume_id.
# The volume has to live in the pod's data center, so the pod is pinned to the
# volume's; the volume is billed for the duration of the run.
run "create_with_network_volume" {
  module {
    source = "../modules/network-volume"
  }

  variables {
    name        = "tf-test-pod-volume"
    size        = 10
    data_center = "EU-RO-1"
  }

  assert {
    condition     = output.id != ""
    error_message = "network volume id is empty"
  }
}

run "mount_network_volume" {
  module {
    source = "../modules/gpu-pod"
  }

  variables {
    gpu_type_id     = var.gpu_type_id
    name            = "tf-test-pod-mounted"
    cloud           = "SECURE"
    disk            = 20
    data_center_ids = ["EU-RO-1"]
    network_volumes = [{
      volume_id = run.create_with_network_volume.id
      path      = "/runpod-volume"
    }]
  }

  assert {
    condition     = length(output.network_volumes) == 1
    error_message = "the network volume was not mounted"
  }
  assert {
    condition     = output.network_volumes[0].volume_id == run.create_with_network_volume.id
    error_message = "the mounted volume id did not round-trip"
  }
  assert {
    condition     = output.network_volumes[0].path == "/runpod-volume"
    error_message = "the mount path did not round-trip"
  }
}
