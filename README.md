# Runpod Terraform Provider

A Terraform provider for managing Runpod infrastructure using the Terraform Plugin Framework.

## Quick Start

### Prerequisites

- Go 1.21 or higher (for development)
- Terraform 1.0 or higher
- Runpod API token

### Development Setup (Recommended)

This provider uses Terraform's `dev_overrides` feature for local development. **No binary building required!**

Create a Terraform CLI config file at `~/.terraform.d/config.tfrc`:

```hcl
provider_installation {
  dev_overrides {
    "runpod/runpod" = "./"
  }
  direct {}
}
```

Then use the provider in your Terraform configuration:

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

### For Production Use

If you need to build and use a binary:

```bash
go build -o terraform-provider-runpod
```

Then configure Terraform to use the local provider binary:

```hcl
provider_installation {
  filesystem_paths {
    paths = ["."]
  }
}
```

## Usage

### Basic Example

```hcl
terraform {
  required_providers {
    runpod = {
      source = "runpod/runpod"
    }
  }
}

# API key can be set via environment variable RUNPOD_API_KEY
# or in the provider configuration (not shown in this example)

resource "runpod_pod" "demo" {
  machine_id  = "your-machine-id"
  image_name  = "runpod/miniconda:py3.10-cuda11.8.0"
  gpu_count   = 1
  start_ssh   = true
}
```

### Environment Variable

Set your Runpod API key as an environment variable:

```bash
export RUNPOD_API_KEY="your-api-key-here"
```

Get your API key from [Runpod Console](https://runpod.io/console/user/settings)

### Examples Directory

- `examples/basic/` - Basic pod creation
- `examples/actions/` - Pod actions
- `examples/datasources/` - Data sources
- `examples/machine/` - Machine management
- `examples/monitoring/` - Pod monitoring

## Development

### Provider Specification

The provider schema is defined in `terraform-provider-spec.json`. To regenerate provider code after modifying the spec:

```bash
tfplugingen-framework generate all \
    --input terraform-provider-spec.json \
    --output internal/provider
```

### Directory Structure

```
terraform-provider-runpod/
├── internal/provider/          # Generated code
│   ├── provider_runpod/
│   ├── resource_pod/
│   ├── resource_pod_action/
│   ├── resource_machine/
│   ├── datasource_pod/
│   ├── datasource_machine/
│   └── ...
├── examples/                   # Example configurations
├── main.go                     # Provider entry point
├── plugin.go                   # Plugin interface
├── go.mod                      # Go dependencies
└── terraform-provider-spec.json # Provider schema definition
```

## API Documentation

- [Runpod API Docs](https://docs.runpod.io)
- [REST API Reference](https://rest.runpod.io/v1/docs)

## Provider Specification

The provider specification is defined in `terraform-provider-spec.json` and includes:

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

## License

MIT
