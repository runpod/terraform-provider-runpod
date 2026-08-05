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

# Create a network volume for persistent storage
resource "runpod_network_volume" "data" {
  name           = var.volume_name
  size           = var.volume_size_gb
  data_center_id = var.data_center_id
  type           = "STANDARD"
}

# Create a template for the training image
resource "runpod_template" "pytorch" {
  name                 = var.template_name
  image_name           = var.image_name
  category             = "NVIDIA"
  is_public            = false
  is_serverless        = false
  container_disk_in_gb = 50

  ports = ["8888/http", "22/tcp"]

  env = {
    PYTHONUNBUFFERED = "1"
    PYTHONDONTWRITEBYTECODE = "1"
  }
}

# Create a pod from the template and attach the network volume
# (volumes attach to pods, not templates)
resource "runpod_pod" "training" {
  name              = "pytorch-training-pod"
  template_id       = runpod_template.pytorch.id
  gpu_count         = var.gpu_count
  cloud_type        = "SECURE"
  network_volume_ids = [runpod_network_volume.data.id]
  volume_mount_path = "/workspace/data"
}

# Output all resource IDs and details
output "network_volume_id" {
  description = "Network volume ID"
  value       = runpod_network_volume.data.id
}

output "network_volume_info" {
  description = "Network volume details"
  value = {
    name           = runpod_network_volume.data.name
    size_gb        = runpod_network_volume.data.size
    data_center_id = runpod_network_volume.data.data_center_id
    type           = runpod_network_volume.data.type
  }
}

output "template_id" {
  description = "Template ID"
  value       = runpod_template.pytorch.id
}

output "template_info" {
  description = "Template details"
  value = {
    name           = runpod_template.pytorch.name
    image_name     = runpod_template.pytorch.image_name
    category       = runpod_template.pytorch.category
    ports          = runpod_template.pytorch.ports
  }
}

output "pod_id" {
  description = "Pod ID"
  value       = runpod_pod.training.id
}

output "pod_info" {
  description = "Pod details"
  value = {
    id        = runpod_pod.training.id
    name      = runpod_pod.training.name
    status    = runpod_pod.training.status
    gpu_count = runpod_pod.training.gpu_count
  }
}

output "pod_ports" {
  description = "Port mappings as returned by the API (ports is a string attribute, e.g. \"22/TCP -> 203.0.113.10:31333|8888/HTTP -> ...\")"
  value       = runpod_pod.training.ports
}

output "pod_logs_url" {
  description = "Pod logs endpoint (v2 REST)"
  value       = format("https://api.runpod.io/v2/pods/%s/logs", runpod_pod.training.id)
}
