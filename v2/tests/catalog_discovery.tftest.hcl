provider "runpod" {}

# Read-only catalog reads, safe against any environment. cpu_type_id is left
# unset so the by-id CPU lookup does not run: flavor identifiers vary by
# account visibility, so no default can be asserted.

run "catalogs" {
  module {
    source = "../modules/catalog-discovery"
  }

  assert {
    condition     = length(output.gpu_types) > 0
    error_message = "catalog returned no GPU types"
  }
  assert {
    condition     = length(output.cpu_types) > 0
    error_message = "catalog returned no CPU types"
  }
  assert {
    condition     = length(output.data_centers) > 0
    error_message = "catalog returned no data centers"
  }
  assert {
    condition     = output.gpu_type.name != ""
    error_message = "gpu type lookup returned empty name"
  }
  assert {
    condition     = output.data_center.region != ""
    error_message = "data center lookup returned empty region"
  }
  assert {
    condition     = output.cpu_type == null
    error_message = "the by-id CPU lookup ran without a cpu_type_id"
  }
  # The public template catalog, official slice.
  assert {
    condition     = length(output.catalog_templates) > 0
    error_message = "the official template catalog came back empty"
  }
  assert {
    condition     = alltrue([for t in output.catalog_templates : t.id != ""])
    error_message = "a catalog template came back with an empty id"
  }
  assert {
    condition     = alltrue([for t in output.catalog_templates : t.public])
    error_message = "the public catalog returned a template that is not public"
  }
}

run "verified_slice" {
  module {
    source = "../modules/catalog-discovery"
  }

  variables {
    catalog_slice = "verified"
  }

  assert {
    condition     = alltrue([for t in output.catalog_templates : t.image != ""])
    error_message = "a verified catalog template came back without an image"
  }
}
