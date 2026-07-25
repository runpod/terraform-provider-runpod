# Runpod Terraform Provider

An official Terraform provider for managing Runpod GPU cloud resources.

## Installation

### From Terraform Registry

```hcl
terraform {
  required_providers {
    runpod = {
      source  = "runpod/runpod"
      version = "~> 0.1.0"
    }
  }
}
```

### For Development (Local Source)

For local development, use the provider directly from source:

```hcl
terraform {
  required_providers {
    runpod = {
      source  = "./"
    }
  }
}
```

See `LOCAL_SETUP.md` for detailed development setup instructions.

## Configuration

```hcl
provider "runpod" {
  api_key   = var.runpod_api_key
  endpoint  = "https://api.runpod.io/graphql"  # Optional, defaults to production
}
```

## Resources

### runpod_pod

Deploy and manage Runpod pods (compute instances).

```hcl
resource "runpod_pod" "example" {
  name                  = "my-pod"
  pod_template_id       = "template-id"
  container_disk_size_gb = 10
  min_memory            = 8
  min_gpu               = 1
  gpu_types             = "RTX,A100"
  cloud_type            = "ALL"
  wait_for_gpu          = true

  env = jsonencode({
    ENVIRONMENT = "production"
  })

  port = "8080:8080"
  command = "python app.py"
}
```

### runpod_machine

Create and manage dedicated GPU machines.

```hcl
resource "runpod_machine" "example" {
  name        = "training-machine"
  description = "Dedicated machine for ML"
  gpu_count   = 4
  gpu_type    = "A100"
  cpu_count   = 32
  memory_gb   = 256
  disk_gb     = 1000
  region      = "US_EAST"
}
```

### runpod_pod_action

Perform actions on pods (stop/resume/restart/terminate).

```hcl
resource "runpod_pod_action" "example" {
  pod_id   = runpod_pod.example.id
  action   = "stop"  # stop, resume, restart, terminate
}
```

## Data Sources

### runpod_gpu_types

List available GPU types.

```hcl
data "runpod_gpu_types" "available" {}
```

### runpod_data_centers

List data centers.

```hcl
data "runpod_data_centers" "available" {}
```

### runpod_machine

Get single machine by ID.

```hcl
data "runpod_machine" "example" {
  machine_id = "machine-id"
}
```

### runpod_machines

List all machines.

```hcl
data "runpod_machines" "available" {}
```

### runpod_pod

Get single pod by ID.

```hcl
data "runpod_pod" "example" {
  pod_id = "pod-id"
}
```

### runpod_user

Get current user info.

```hcl
data "runpod_user" "current" {}
```



## Authentication

The provider uses API key authentication. Get your API key from the Runpod console at https://www.runpod.io/console/user/settings.

## Development

### Quick Start (dev_overrides)

For local development without building a binary:

```bash
# Create Terraform CLI config
mkdir -p ~/.terraform.d
cat > ~/.terraform.d/config.tfrc << 'EOF'
provider_installation {
  dev_overrides {
    "runpod/runpod" = "./"
  }
  direct {}
}
EOF

# Initialize and run
terraform init
terraform plan
terraform apply
```

### Building from Source

```bash
# Build the provider
go build -o terraform-provider-runpod

# Test
go test ./...
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

See `LOCAL_SETUP.md` for development setup instructions.

## License

This project is licensed under the MIT License.
