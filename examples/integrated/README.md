# Integrated Example: Pod + Template + Network Volume

This example demonstrates a complete workflow:

1. **Network Volume** - Creates persistent storage in your chosen data center
2. **Template** - Creates a reusable configuration with your image and mounts the network volume
3. **Pod** - Launches a pod using the template with network volume attached

## Prerequisites

- RunPod API key (get from https://runpod.io/console/user/settings)
- A data center ID (get from `examples/datasources`)
- A Docker image (defaults to `runpod/pytorch:2.0.1`)

## Usage

```bash
cd examples/integrated

# Initialize
terraform init

# Preview
terraform plan -var="runpod_api_key=your-api-key"

# Apply
terraform apply -var="runpod_api_key=your-api-key"

# Monitor pod status
terraform apply -var="runpod_api_key=your-api-key" -var="volume_name=my-volume" -var="image_name=your-image"
```

## Outputs

After applying, you'll see:

- **Network Volume ID** - ID of the created persistent storage
- **Template ID** - ID of the created template
- **Pod ID** - ID of the launched pod
- **SSH connection info** - Port and command to SSH into the pod
- **Jupyter connection info** - URL for Jupyter access
- **Pod logs URL** - URL to stream pod logs

## What Gets Created

```
Network Volume (persistent storage)
  └─> Template (configuration)
        └─> Pod (running instance)
```

## Customization

Modify `variables.tf` or pass variables:

```bash
terraform apply \
  -var="runpod_api_key=your-key" \
  -var="template_name=my-template" \
  -var="image_name=my-image:latest" \
  -var="volume_size_gb=100" \
  -var="data_center_id=US-MD-1" \
  -var="gpu_count=2"
```

## Notes

- Network volumes are billable persistent resources
- Templates are reusable configurations
- Pods can be stopped/terminated via `examples/actions/`
- The network volume is mounted at `/workspace/data` inside the pod
