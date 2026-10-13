provider "runpod" {}

# Stores a real registry credential (free to keep, deleted on destroy) and, if
# an ECR ARN is supplied, a real delegation, then reads both back out of the
# account-wide listings. All three values are real external input and come from
# the environment, as tests/main.tf describes:
#
#   export TF_VAR_registry_username=... TF_VAR_registry_password=...
#   export TF_VAR_ecr_resource_arn=arn:aws:ecr:us-east-1:123456789012:repository/trainers
#
# The credential cannot be created without the login, so the run fails without
# it. The ARN has to be one this account is allowed to delegate; leaving it
# unset skips the delegation and its assertions.
#
# The run block reads them as var.*, which a test file can only do for
# variables it declares itself; tests/main.tf's declarations do not reach a run
# that loads another module.
variable "registry_username" {
  type    = string
  default = null
}

variable "registry_password" {
  type      = string
  sensitive = true
  default   = null
}

variable "ecr_resource_arn" {
  type    = string
  default = null
}

run "store" {
  module {
    source = "../modules/registry"
  }

  variables {
    name             = "tf-test-registry"
    username         = var.registry_username
    password         = var.registry_password
    delegation_name  = "tf-test-delegation"
    ecr_resource_arn = var.ecr_resource_arn
  }

  assert {
    condition     = output.id != ""
    error_message = "registry id is empty"
  }
  assert {
    condition     = contains(output.registry_ids, output.id)
    error_message = "the new credential is missing from the registries listing"
  }
  assert {
    condition     = alltrue([for id in output.registry_ids : id != ""])
    error_message = "registries listing returned an entry with an empty id"
  }
  assert {
    condition     = var.ecr_resource_arn == null || output.delegation_id != null
    error_message = "an ECR ARN was supplied but no delegation was created"
  }
  assert {
    condition     = var.ecr_resource_arn == null || contains(output.delegation_ids, output.delegation_id)
    error_message = "the new delegation is missing from the delegations listing"
  }
  assert {
    condition     = alltrue([for d in output.delegations : d.id != ""])
    error_message = "delegations listing returned an entry with an empty id"
  }
  assert {
    condition     = alltrue([for d in output.delegations : d.aws_region != "" && d.aws_user != ""])
    error_message = "a delegation came back without the AWS region and principal to authorize"
  }
  assert {
    condition     = alltrue([for d in output.delegations : d.created_at != ""])
    error_message = "a delegation came back without a creation timestamp"
  }
}
