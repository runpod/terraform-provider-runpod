terraform {
  required_version = ">= 1.6"

  required_providers {
    runpod = {
      source = "runpod/runpod"
    }
  }
}

provider "runpod" {}
