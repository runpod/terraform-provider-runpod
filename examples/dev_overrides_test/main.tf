terraform {
  required_providers {
    runpod = {
      source = "runpod/runpod"
    }
  }
}

provider "runpod" {
  api_key = "test-key"
}

resource "runpod_pod" "demo" {
  machine_id  = "test-machine-id"
  image_name  = "runpod/miniconda:py3.10-cuda11.8.0"
  gpu_count   = 1
}

output "pod_id" {
  value = runpod_pod.demo.id
}
