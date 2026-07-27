# Template Example

This example demonstrates how to create and manage Runpod templates.

## What's a Template?

Runpod **templates** are reusable configurations that define:
- Container image to use
- GPU/CPU resources
- Environment variables
- Volume mounts
- Ports to expose
- Docker entrypoint and start commands

Templates can be used to:
- Create consistent Pod deployments
- Power Serverless endpoints
- Share configurations with other users (public templates)

## Configuration

The example shows:
- `name`: Template name (must be unique)
- `image_name`: Docker image to use
- `category`: NVIDIA, AMD, or CPU
- `is_public`: Whether other users can use it
- `is_serverless`: Whether it's for serverless endpoints
- `container_disk_in_gb`: Container disk size
- `volume_in_gb`: Volume size for persistence
- `volume_mount_path`: Where volume mounts

## Usage

```bash
cd examples/template

# Set your API key
export RUNPOD_API_KEY="your-api-key-here"

# Create the template
terraform apply

# Get the template ID
terraform output template_id

# Use the template ID to create a pod or endpoint
# pod: template_id = runpod_template.demo.id
# endpoint: template_id = runpod_template.demo.id

# Clean up
terraform destroy
```

## Template Usage

Once created, templates can be used with:

### Pods
```hcl
resource "runpod_pod" "from_template" {
  template_id = runpod_template.demo.id
  # other pod config...
}
```

### Endpoints
```hcl
resource "runpod_endpoint" "from_template" {
  template_id = runpod_template.demo.id
  # other endpoint config...
}
```

## Notes

- Template names must be unique per user
- Public templates can be used by any Runpod user
- Serverless templates power endpoint workers
- Pod templates create dedicated GPU instances
