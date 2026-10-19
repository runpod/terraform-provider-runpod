terraform {
  required_version = ">= 1.6"

  required_providers {
    runpod = {
      source  = "runpod/runpod"
      version = "2.0.0-rc.3"
    }
  }
}

# Reads the API key from RUNPOD_API_KEY.
provider "runpod" {}

# A template is a saved image and disk configuration that pods and Serverless
# endpoints start from. Creating one starts no pod.
resource "runpod_template" "example" {
  name  = "terraform-example"
  image = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  disk  = 20
}

output "template_id" {
  value = runpod_template.example.id
}
