# Root configuration for `terraform test`. The test files in this directory
# call the use-case modules under modules/, so every module doubles as an
# acceptance test. Credentials come from RUNPOD_API_KEY / RUNPOD_API_BASE_URL.

terraform {
  required_version = ">= 1.6"

  required_providers {
    runpod = {
      source  = "runpod/runpod"
      version = "2.0.0-rc.3"
    }
  }
}

provider "runpod" {}

# The registry test needs real external input, a registry login and an ECR ARN
# this account is allowed to delegate, so it comes from the environment rather
# than a checked-in default:
#
#   export TF_VAR_registry_username=... TF_VAR_registry_password=...
#   export TF_VAR_ecr_resource_arn=arn:aws:ecr:us-east-1:123456789012:repository/trainers
#
# Without the ARN the delegation half of that test is skipped; without the
# login the credential cannot be created and the run fails.
variable "registry_username" {
  description = "Registry login for the registry test. TF_VAR_registry_username."
  type        = string
  default     = null
}

variable "registry_password" {
  description = "Registry password or token for the registry test. TF_VAR_registry_password."
  type        = string
  sensitive   = true
  default     = null
}

variable "ecr_resource_arn" {
  description = "AWS ECR resource ARN to delegate pulls for. TF_VAR_ecr_resource_arn; null skips the delegation."
  type        = string
  default     = null
}

# The GPU for the pod tests. L40S has had the steadiest Secure Cloud stock;
# override it when the catalog shows none: TF_VAR_gpu_type_id, or E2E_GPU_TYPE
# through task test-e2e.
variable "gpu_type_id" {
  description = "GPU type the pod tests create. TF_VAR_gpu_type_id."
  type        = string
  default     = "NVIDIA L40S"
}
