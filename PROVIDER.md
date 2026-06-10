# RunPod Terraform Provider

A Terraform provider for managing RunPod infrastructure.

## Overview

This provider allows you to manage RunPod resources using Terraform, including:

- Creating and managing pods
- Performing pod actions (stop, resume, terminate, reset)
- Managing machine listings and bidding
- Querying available resources (GPU types, data centers, machines)

## Provider Configuration

```hcl
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
```

## Development Setup

For local development without building a binary, see `LOCAL_SETUP.md`.

## Resources

### runpod_pod

Create and manage RunPod pods.

**Arguments:**

- `machine_id` (Required) - Machine ID to deploy pod on
- `image_name` (Required) - Docker image name
- `name` (Optional) - Pod name
- `gpu_count` (Optional, default=1) - Number of GPUs
- `gpu_type_id` (Optional) - Specific GPU type ID
- `cloud_type` (Optional, default="COMMUNITY") - Cloud type: COMMUNITY, SECURE, or ALL
- `docker_args` (Optional) - Docker arguments
- `env` (Optional) - Environment variables
- `port` (Optional) - Main port for the pod
- `ports` (Optional) - Port configuration string
- `volume_in_gb` (Optional) - Volume size in GB
- `volume_key` (Optional, sensitive) - Volume encryption key
- `volume_mount_path` (Optional) - Volume mount path
- `container_disk_in_gb` (Optional) - Container disk size
- `template_id` (Optional) - Pod template ID
- `start_ssh` (Optional, default=false) - Start SSH on boot
- `start_jupyter` (Optional, default=false) - Start Jupyter on boot
- `bid_per_gpu` (Optional) - Bid price per GPU

**Attributes:**

- `id` - Pod ID
- `status` - Current pod status
- `cost_per_hr` - Cost per hour
- `created_at` - Creation timestamp
- `gpu_type_id` - GPU type ID
- `machine_id` - Machine ID
- And more...

### runpod_pod_action

Perform actions on pods.

**Arguments:**

- `pod_id` (Required) - Pod ID to act on
- `action` (Required) - Action: stop, resume, terminate, reset

**Attributes:**

- `status` - Status after action

### runpod_machine

Manage machine listings.

**Arguments:**

- `gpu_type_id` (Required) - GPU type ID
- `name` (Optional) - Machine name
- `gpu_count` (Optional, default=1) - Number of GPUs
- `cpu_count` (Optional) - Number of CPUs
- `memory_in_gb` (Optional) - Memory in GB
- `disk_in_gb` (Optional) - Disk size in GB
- `data_center_id` (Optional) - Data center ID
- `secure_cloud` (Optional, default=false) - Use secure cloud
- `listed` (Optional, default=true) - List machine

## Data Sources

### runpod_pod

Retrieve pod information.

**Arguments:**

- `id` (Required) - Pod ID

### runpod_machine

Retrieve machine information.

**Arguments:**

- `id` (Required) - Machine ID

### runpod_machines

List all machines.

**Arguments:**

- `listed` (Optional) - Filter by listed status

### runpod_gpu_types

List available GPU types.

### runpod_data_centers

List available data centers.

### runpod_user

Get current user information.

## Examples

See the `examples/` directory for complete examples:

- `examples/basic/` - Basic pod creation
- `examples/actions/` - Pod actions (stop, resume, terminate)
- `examples/datasources/` - Using data sources
- `examples/machine/` - Machine bidding
- `examples/monitoring/` - Pod monitoring

## Development

See `LOCAL_SETUP.md` for development setup instructions.
