# PyTorch Pod Example

Create a pod from a pre-created RunPod template using the v2 API — the provider fetches the template's image, ports, env, args, disk, and mounts.

## Prerequisites

1. Get your RunPod API key from https://runpod.io/console/user/settings
2. Get a Template ID from https://www.runpod.io/console/templates

## Usage

```bash
cd examples/pytorch
export RUNPOD_API_KEY="your-api-key-here"
terraform init
terraform plan -var="template_id=your-template-id"
```

## Notes

- This example is for documentation purposes only
- Pod creation may take several minutes once applied

## Outputs

- `pod_id`: The pod identifier
- `pod_status`: Current pod status
