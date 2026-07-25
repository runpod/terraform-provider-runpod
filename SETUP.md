# Runpod Terraform Provider Setup

This document provides setup instructions for using the Runpod Terraform provider.

## Quick Start (Recommended for Development)

This provider supports Terraform's `dev_overrides` feature, which allows you to use the provider directly from source without building a binary.

### 1. Configure Terraform CLI

Create `~/.terraform.d/config.tfrc`:

```hcl
provider_installation {
  dev_overrides {
    "runpod/runpod" = "./"
  }
  direct {}
}
```

### 2. Update Example Configuration

Edit `examples/basic/variables.tf` and replace:
- `YOUR_API_KEY_HERE` with your Runpod API key
- `YOUR_MACHINE_ID_HERE` with a machine ID from your Runpod account

### 3. Run the Demo

```bash
cd examples/basic
terraform init
terraform apply
```

## Alternative: Using a Built Binary

If you prefer to build and use a provider binary:

### 1. Build the Provider

```bash
go build -o terraform-provider-runpod
```

### 2. Configure Terraform

```hcl
provider_installation {
  filesystem_paths {
    paths = ["."]
  }
}
```

### 3. Run the Demo

```bash
cd examples/basic
terraform init
terraform apply
```

## Detailed Setup

### Get Runpod API Key

1. Visit https://runpod.io/console/user/settings
2. Copy your API key
3. Store it securely (use environment variable or update variables.tf)

### Find Available Machines

Use the datasources demo to find available machines:

```bash
cd examples/datasources
terraform apply -var="runpod_api_key=your-key"
```

This will list all available machines you can deploy pods to.

### Create a Pod

Once you have a machine ID:

```bash
cd examples/basic
terraform apply \
  -var="runpod_api_key=your-key" \
  -var="machine_id=your-machine-id"
```

### Monitor Your Pod

```bash
cd examples/monitoring
terraform apply \
  -var="runpod_api_key=your-key" \
  -var="pod_id=the-pod-id-from-previous-step"
```

### Clean Up

```bash
cd examples/actions
terraform apply \
  -var="runpod_api_key=your-key" \
  -var="pod_id=your-pod-id" \
  -var="action=terminate"
```

## Troubleshooting

### Provider Not Found

- For `dev_overrides`: Ensure you're in the provider directory or adjust the path in `config.tfrc`
- For binary approach: Ensure the provider binary is in your current directory and Terraform config is correct

### No Machines Available

You need to list machines in your Runpod console before they appear in the API.

### Authentication Errors

Verify your API key is correct and has the necessary permissions.

## Next Steps

- Explore other examples in the `examples/` directory
- Modify the provider specification in `terraform-provider-spec.json`
- Implement custom logic in the generated provider code
- Contribute to the provider on GitHub
