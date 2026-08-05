# Endpoint Job Example

This example demonstrates how to run a job on a RunPod endpoint.

## Prerequisites

1. Get your RunPod API key from https://runpod.io/console/user/settings
2. Have an existing endpoint ID (create one using the `runpod_endpoint` resource or the console)

## Usage

```bash
cd examples/endpoint_job
terraform init
terraform apply -var="runpod_api_key=your-api-key" -var="endpoint_id=your-endpoint-id" -var="input={\"prompt\":\"hello world\"}"
```

## What it creates

- Runs a job on the specified endpoint
- Outputs include job ID, status, output, and timing information

## Notes

- The `input` field should be a JSON string matching your endpoint's expected input format
- For serverless endpoints, you may need to specify `template_id`
