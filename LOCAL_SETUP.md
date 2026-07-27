# Runpod Terraform Provider - Development Setup

This provider uses the Terraform Plugin Framework and supports local development without building a binary.

## Quick Start (No Binary Needed)

### Option 1: Using dev_overrides (Recommended)

Create a Terraform CLI config file at `~/.terraform.d/config.tfrc`:

```hcl
provider_installation {
  dev_overrides {
    "runpod/runpod" = "./"
  }
  direct {}
}
```

Then in your Terraform configuration:

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

### Option 2: Using Local Mirror

Copy the provider directory to a local mirror location and configure:

```hcl
provider_installation {
  filesystem_mirror {
    "runpod/runpod" = ["/path/to/provider/directory"]
  }
  direct {}
}
```

## For This Repository

This repository contains:

1. **Generated Provider Code** (`internal/provider/`)
   - All resources and data sources generated from `terraform-provider-spec.json`
   - No manual implementation needed for basic schema

2. **Example Configurations** (`examples/`)
   - Demonstrates how to use each resource and data source

3. **Provider Spec** (`terraform-provider-spec.json`)
   - Defines all resources, data sources, and their schemas
   - Can be modified to add new features

## Running Examples

```bash
# Initialize Terraform (uses dev_overrides from ~/.terraform.d/config.tfrc)
cd examples/basic
terraform init

# Plan with your API key
terraform plan -var="runpod_api_key=your-key" -var="machine_id=your-machine-id"

# Apply
terraform apply -var="runpod_api_key=your-key" -var="machine_id=your-machine-id"
```

## Provider Specification

The provider is defined in `terraform-provider-spec.json` with:

### Resources
- `runpod_pod` - Pod management
- `runpod_pod_action` - Pod actions
- `runpod_machine` - Machine management

### Data Sources
- `runpod_pod` - Pod information
- `runpod_machine` - Machine information
- `runpod_machines` - Machine listing
- `runpod_gpu_types` - GPU types
- `runpod_data_centers` - Data centers
- `runpod_user` - User info

## Regenerating Provider Code

If you modify `terraform-provider-spec.json`:

```bash
tfplugingen-framework generate all \
    --input terraform-provider-spec.json \
    --output internal/provider
```

## Publishing to Terraform Registry

To make this truly plug-and-play for others:

1. Register at https://registry.terraform.io/
2. Create a namespace (e.g., `runpod`)
3. Set up GitHub integration
4. Tag releases with `vX.Y.Z` format
5. The provider will be available as `runpod/runpod`

Once published, users can simply use:

```hcl
terraform {
  required_providers {
    runpod = {
      source = "runpod/runpod"
      version = ">=0.1.0"
    }
  }
}
```
