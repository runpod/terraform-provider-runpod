# Endpoint Example

This example demonstrates how to create and manage Runpod Serverless endpoints.

## What's an Endpoint?

Runpod **endpoints** are serverless compute resources that:
- Automatically scale workers based on demand
- Spin down when idle (no charges when no work is being processed)
- Use templates to define the container image and configuration
- Support multiple workers for parallel processing
- Can attach network volumes for shared data storage

## Key Features

- **Auto-scaling**: Workers scale up/down based on queue size or request count
- **Cost-effective**: Only pay for active workers
- **Idle timeout**: Workers shut down after period of inactivity
- **Network volumes**: Attach persistent storage to workers

## Configuration

The example shows:
- `template_id`: Template to use (required)
- `name`: Endpoint name
- `workers_min/max`: Scale boundaries
- `idle_timeout`: Seconds before scaling down
- `gpu_count`: GPUs per worker

## Usage

```bash
cd examples/endpoint

# Set your API key
export RUNPOD_API_KEY="your-api-key-here"

# Create the endpoint
terraform apply

# Get the endpoint URL for API calls
terraform output endpoint_url

# Clean up
terraform destroy
```

## API Call Format

Once created, call your endpoint at:
```
https://api.runpod.ai/v2/{endpoint_id}/runsync
```

Example request body:
```json
{
  "input": {
    "prompt": "Your input here"
  }
}
```

## Notes

- Endpoints require a template to be created first
- Workers automatically scale based on `workers_min` and `workers_max`
- Network volumes can be attached for shared storage across workers
