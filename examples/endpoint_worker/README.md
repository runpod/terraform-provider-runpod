# Endpoint Worker Example

This example demonstrates how to retrieve information about an endpoint worker.

## Prerequisites

1. Get your RunPod API key from https://runpod.io/console/user/settings
2. Have an existing endpoint ID with active workers (create one using the `runpod_endpoint` resource or the console)

## Usage

```bash
cd examples/endpoint_worker
terraform init
terraform apply -var="runpod_api_key=your-api-key" -var="endpoint_id=your-endpoint-id"
```

## What it retrieves

- Worker details including pod ID, container ID, status, uptime, and last busy time
- Note: This resource reads existing worker information; it does not create new workers

## Notes

- Workers are automatically created when you run jobs on an endpoint
- Use this to monitor worker activity and status
