# Container Registry Auth Example

This example demonstrates how to create and manage container registry authentication for Runpod.

## What's Container Registry Auth?

Runpod **container registry auth** allows you to:
- Authenticate with private Docker registries (Docker Hub, ECR, GCR, etc.)
- Use private images in your Pods and Serverless endpoints
- Store credentials securely in Runpod's infrastructure

## Configuration

The example shows:
- `name`: Authentication name (must be unique)
- `username`: Registry username
- `password`: Registry password or access token (sensitive)

## Usage

```bash
cd examples/container_registry_auth

# Set your API key
export RUNPOD_API_KEY="your-api-key-here"

# Create the authentication
terraform apply

# Get the auth ID
terraform output auth_id

# Use the auth ID in your Pod or Endpoint configuration:
# container_registry_auth_id = runpod_container_registry_auth.demo.id

# Clean up
terraform destroy
```

## Using with Pods

```hcl
resource "runpod_pod" "with_private_image" {
  container_registry_auth_id = runpod_container_registry_auth.demo.id
  image_name                 = "private-registry.com/my-image:latest"
  # other config...
}
```

## Using with Endpoints

```hcl
resource "runpod_endpoint" "with_private_image" {
  container_registry_auth_id = runpod_container_registry_auth.demo.id
  template_id                = var.template_id
  # other config...
}
```

## Notes

- Authentication names must be unique per user
- Passwords are stored securely by Runpod
- You can use personal access tokens instead of passwords for registries like Docker Hub
