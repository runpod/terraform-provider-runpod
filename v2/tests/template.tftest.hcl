provider "runpod" {}

run "create" {
  module {
    source = "../modules/template"
  }

  variables {
    name = "tf-test-template"
  }

  assert {
    condition     = output.id != ""
    error_message = "template id is empty"
  }
  assert {
    condition     = output.image == "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
    error_message = "template image does not round-trip"
  }
  assert {
    condition     = output.persistent_volume == null
    error_message = "a template with no mount configured must report none"
  }
  assert {
    condition     = output.start_ssh != null && output.start_jupyter != null
    error_message = "the API defaults for the SSH and Jupyter flags were not read back"
  }
  # The by-id read: what a pod or endpoint pointing at the template sees.
  assert {
    condition     = output.lookup_image == output.image
    error_message = "the runpod_template data source read back a different image"
  }
  assert {
    condition     = output.lookup_start_ssh == output.start_ssh
    error_message = "the runpod_template data source read back a different SSH flag"
  }
  assert {
    condition     = contains(output.template_ids, output.id)
    error_message = "the new template is missing from the templates listing"
  }
  assert {
    condition     = alltrue([for id in output.template_ids : id != ""])
    error_message = "templates listing returned an entry with an empty id"
  }
  # The deprecated REST v1 shim reads back a private template's counters. They
  # can be null for a template created through v2 (a failed shim read is a
  # warning and a null); when present they must be the fresh values.
  assert {
    condition     = output.is_runpod == null || output.is_runpod == false
    error_message = "a template created by this account must not be reported as a Runpod one"
  }
  assert {
    condition     = (output.earned == null || output.earned == 0) && (output.runtime_in_min == null || output.runtime_in_min == 0)
    error_message = "a fresh template must report no earnings and no runtime"
  }
}

# The container settings the resource gained: a persistent mount, the startup
# flags, and a CUDA constraint, all in one create.
run "create_with_container_settings" {
  module {
    source = "../modules/template"
  }

  variables {
    name                  = "tf-test-template-full"
    start_ssh             = true
    start_jupyter         = false
    allowed_cuda_versions = ["12.8"]
    persistent_volume = {
      size = 20
      path = "/workspace"
    }
    # Applied through the deprecated REST v1 shim, then read back from it.
    docker_entrypoint = ["/bin/bash"]
    readme            = "created by the acceptance test"
  }

  assert {
    condition     = output.start_ssh == true && output.start_jupyter == false
    error_message = "the SSH and Jupyter flags did not round-trip"
  }
  assert {
    condition     = output.persistent_volume.size == 20 && output.persistent_volume.path == "/workspace"
    error_message = "the persistent mount did not round-trip"
  }
  assert {
    condition     = length(output.allowed_cuda_versions) == 1 && output.allowed_cuda_versions[0] == "12.8"
    error_message = "the CUDA constraint did not round-trip"
  }
  assert {
    condition     = length(output.docker_entrypoint) == 1 && output.docker_entrypoint[0] == "/bin/bash"
    error_message = "the entrypoint did not round-trip through the REST v1 shim"
  }
  assert {
    condition     = output.readme == "created by the acceptance test"
    error_message = "the readme did not round-trip through the REST v1 shim"
  }
  assert {
    condition     = output.lookup_docker_entrypoint[0] == "/bin/bash"
    error_message = "the runpod_template data source did not read the entrypoint back from the v2 template"
  }
}

# In-place update of the same settings: the mount path and the flags are all
# patchable, so this must not replace the template.
run "update_container_settings" {
  module {
    source = "../modules/template"
  }

  variables {
    name                  = "tf-test-template-full"
    start_ssh             = false
    start_jupyter         = true
    allowed_cuda_versions = ["12.6"]
    persistent_volume = {
      size = 30
      path = "/data"
    }
    docker_entrypoint = ["/bin/sh"]
    readme            = "updated by the acceptance test"
  }

  assert {
    condition     = output.start_ssh == false && output.start_jupyter == true
    error_message = "in-place flag update did not round-trip"
  }
  assert {
    condition     = output.persistent_volume.size == 30 && output.persistent_volume.path == "/data"
    error_message = "in-place mount update did not round-trip"
  }
  assert {
    condition     = output.allowed_cuda_versions[0] == "12.6"
    error_message = "in-place CUDA constraint update did not round-trip"
  }
  assert {
    condition     = output.docker_entrypoint[0] == "/bin/sh"
    error_message = "in-place entrypoint update did not round-trip through the REST v1 shim"
  }
  assert {
    condition     = output.readme == "updated by the acceptance test"
    error_message = "in-place readme update did not round-trip through the REST v1 shim"
  }
}
