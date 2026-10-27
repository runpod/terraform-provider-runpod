provider "runpod" {}

# Creates a real global volume, renames it in place and reads it back by id;
# terraform test destroys it afterwards. An empty global volume stores
# nothing, so it bills nothing.

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
