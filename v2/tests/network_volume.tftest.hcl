provider "runpod" {}

# Creates a real (billed) volume and reads it back out of the account-wide
# listing; terraform test destroys it afterwards.

run "create" {
  module {
    source = "../modules/network-volume"
  }

  variables {
    name = "tf-test-volume"
    size = 10
  }

  assert {
    condition     = output.id != ""
    error_message = "volume id is empty"
  }
  assert {
    condition     = output.size == 10
    error_message = "volume size does not match the requested 10 GB"
  }
  assert {
    condition     = contains(["STANDARD", "HIGH_PERFORMANCE"], output.type)
    error_message = "volume type not populated from the API default"
  }
  assert {
    condition     = contains(output.network_volume_ids, output.id)
    error_message = "the new volume is missing from the network volumes listing"
  }
  assert {
    condition     = alltrue([for id in output.network_volume_ids : id != ""])
    error_message = "network volumes listing returned an entry with an empty id"
  }
}
