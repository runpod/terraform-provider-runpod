# RunPod Terraform Provider Demo

This demo shows how to use the RunPod Terraform provider to manage RunPod resources.

## Prerequisites

- Terraform 1.0 or higher
- RunPod API token (get from https://runpod.io/console/user/settings)

## Development Setup

### Option 1: Using dev_overrides (Recommended)

```bash
mkdir -p ~/.terraform.d
cat > ~/.terraform.d/config.tfrc << 'EOF'
provider_installation {
  dev_overrides {
    "runpod/runpod" = "./"
  }
  direct {}
}
EOF
```

### Option 2: Using Built Binary

```bash
go build -o terraform-provider-runpod

# Configure Terraform to use local binary
cat > ~/.terraform.rc << 'EOF'
provider_installation {
  filesystem_paths {
    paths = ["."]
  }
}
EOF
```

See `LOCAL_SETUP.md` for more details.

## Example Usage

### Basic Pod Creation

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

resource "runpod_pod" "demo_pod" {
  machine_id   = "your-machine-id"
  image_name   = "runpod/miniconda:py3.10-cuda11.8.0"
  gpu_count    = 1
  name         = "demo-pod"
  start_ssh    = true
  start_jupyter = true
}

output "pod_id" {
  value = runpod_pod.demo_pod.id
}

output "pod_status" {
  value = runpod_pod.demo_pod.status
}
```

### Using Data Sources

```hcl
# List available GPU types
data "runpod_gpu_types" "available" {}

# List available data centers
data "runpod_data_centers" "locations" {}

# Get current user info
data "runpod_user" "current" {}

# List available machines
data "runpod_machines" "available" {
  listed = true
}

# Get specific machine
data "runpod_machine" "selected" {
  id = "machine-id-here"
}

# Get specific pod
data "runpod_pod" "running" {
  id = "pod-id-here"
}
```

### Pod Actions

```hcl
resource "runpod_pod_action" "stop_pod" {
  pod_id = "pod-id-here"
  action = "stop"
}

resource "runpod_pod_action" "resume_pod" {
  pod_id = "pod-id-here"
  action = "resume"
}

resource "runpod_pod_action" "terminate_pod" {
  pod_id = "pod-id-here"
  action = "terminate"
}
```

### Machine Bidding

```hcl
resource "runpod_machine" "bid_machine" {
  gpu_type_id     = "RTX 3090"
  gpu_count       = 2
  cpu_count       = 8
  memory_in_gb    = 32
  disk_in_gb      = 50
  data_center_id  = "data-center-id"
  secure_cloud    = false
  listed          = true
  host_price_per_gpu = 0.50
}
```

## Environment Variables

Set these environment variables instead of using provider config:

```bash
export RUNPOD_API_KEY="your-api-key"
```

## Available Resources

- `runpod_pod` - Create and manage pods
- `runpod_pod_action` - Perform actions on pods (stop, resume, terminate, reset)
- `runpod_machine` - Manage machine listings and bidding

## Available Data Sources

- `runpod_pod` - Retrieve pod information
- `runpod_machine` - Retrieve machine information
- `runpod_machines` - List all machines
- `runpod_gpu_types` - List available GPU types
- `runpod_data_centers` - List data centers
- `runpod_user` - Get current user info

## Running the Demo

1. Set up your Terraform configuration with your API key
2. Run: `terraform init && terraform plan && terraform apply`
