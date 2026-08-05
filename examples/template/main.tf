terraform {
  required_providers {
    runpod = {
      source = "runpod/runpod"
    }
  }
}

provider "runpod" {
  api_key = var.runpod_api_key
}

resource "runpod_template" "demo" {
  name                 = var.template_name
  image_name           = var.image_name
  category             = var.category
  is_public            = var.is_public
  is_serverless        = var.is_serverless
  container_disk_in_gb = var.container_disk_in_gb
  volume_in_gb         = var.volume_in_gb
  volume_mount_path    = var.volume_mount_path
}

output "template_id" {
  description = "The template ID"
  value       = runpod_template.demo.id
}

output "template_info" {
  description = "Template details"
  value = {
    name         = runpod_template.demo.name
    image_name   = runpod_template.demo.image_name
    category     = runpod_template.demo.category
    is_public    = runpod_template.demo.is_public
    is_serverless = runpod_template.demo.is_serverless
  }
}
