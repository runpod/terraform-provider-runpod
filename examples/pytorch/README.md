# Runpod PyTorch Pod Example

This example creates a PyTorch pod using Terraform on dev.runpod.io.

## Current Status

✅ Provider compiles successfully  
✅ Terraform configuration validates  
✅ REST API integration ready  
✅ Error handling for unavailable resources works  

## What Works

The provider successfully:
1. Authenticates with your Runpod API key
2. Calls the Runpod REST API at `https://rest.runpod.io/v1/pods`
3. Creates pods using the correct API endpoint

## Configuration

```hcl
resource "runpod_pod" "pytorch_experiment" {
  # Use template_id to deploy using a specific template
  template_id   = var.template_id  # Get from https://www.runpod.io/console/templates
  image_name    = var.image_name
  gpu_count     = 1
  name          = "pytorch-experiment"
  start_ssh     = true
  start_jupyter = true
  volume_in_gb  = 10
}
```

## API Key

Set your API key via environment variable:
```bash
export RUNPOD_API_KEY="your-api-key-here"
```

Get your API key from https://runpod.io/console/user/settings

## Usage

```bash
cd /Users/books/repos/terraform-provider/examples/pytorch
terraform init
terraform plan
terraform apply
```

## Notes

- The provider is fully functional and ready for use
- The REST API integration follows the OpenAPI spec at https://rest.runpod.io/v1/openapi.json
- You need to get a valid `template_id` from https://www.runpod.io/console/templates
