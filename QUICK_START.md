# Runpod Terraform Provider - Development Setup

This provider uses Terraform's `dev_overrides` feature for local development - **no binary building required**!

## Quick Start

### 1. Create Terraform CLI Config

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

### 2. Use in Your Terraform Configuration

```hcl
terraform {
  required_providers {
    runpod = {
      source = "runpod/runpod"
    }
  }
}

provider "runpod" {
  api_key = "your-api-key-here"
}

resource "runpod_pod" "demo" {
  machine_id  = "your-machine-id"
  image_name  = "runpod/miniconda:py3.10-cuda11.8.0"
  gpu_count   = 1
  start_ssh   = true
}
```

### 3. Run Terraform

```bash
terraform init
terraform plan
terraform apply
```

## How It Works

The provider code is in `internal/provider/`:
- All resources and data sources are generated from `terraform-provider-spec.json`
- The `main.go` implements the provider interface
- Terraform loads the code directly via `dev_overrides` - **no binary needed!**

## Regenerating Provider Code

If you modify `terraform-provider-spec.json`:

```bash
export PATH="$HOME/go/bin:$PATH"
tfplugingen-framework generate all \
    --input terraform-provider-spec.json \
    --output internal/provider
```

## What's Included

### Resources
- `runpod_pod` - Create and manage pods
- `runpod_pod_action` - Pod actions (stop, resume, terminate, reset)
- `runpod_machine` - Machine management

### Data Sources
- `runpod_pod` - Pod information
- `runpod_machine` - Machine information
- `runpod_machines` - List machines
- `runpod_gpu_types` - GPU types
- `runpod_data_centers` - Data centers
- `runpod_user` - User info

## Publishing to Terraform Registry

To make this truly plug-and-play for others:

1. Register at https://registry.terraform.io/
2. Create a `runpod` namespace
3. Push to GitHub
4. Tag releases: `v0.1.0`, `v0.2.0`, etc.
5. Users can then use: `source = "runpod/runpod"`
