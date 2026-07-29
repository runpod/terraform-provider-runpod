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

The provider schema is defined in `terraform-provider-spec.json`, which documents the
logical schema of every resource and data source. Regeneration produces semantically
equivalent schema code:

```bash
tfplugingen-framework generate all \
    --input terraform-provider-spec.json \
    --output internal/provider
```

Notes:

- The 8 data sources with `list_nested` schemas (`data_centers`, `gpu_types`,
  `machines`, `container_registry_auth`, `ecr_delegations`, and the three `billing_*`
  sources) are marked hand-maintained in their `_gen.go` headers: the installed
  generator emits custom types for nested attributes, while these files use plain
  model slices. Update their schema by hand and mirror the change into the spec.
- The spec is current with the provider's shipped schemas; treat it as the source of
  truth for attribute-level changes to all other components.

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
