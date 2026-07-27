# ECR Delegation Example

This example demonstrates how to create an ECR (Elastic Container Registry) delegation to allow RunPod to access your private ECR repositories.

## Prerequisites

1. Get your RunPod API key from https://runpod.io/console/user/settings
2. Have an ECR resource ARN from AWS (format: `arn:aws:ecr:region:account-id:repository/repository-name`)

## Usage

```bash
cd examples/ecr_delegation
terraform init
terraform apply -var="runpod_api_key=your-api-key" -var="ecr_resource_arn=your-ecr-arn"
```

## What it creates

- ECR delegation allowing RunPod to pull images from your private ECR repository
- Outputs include delegation details like AWS user, repository name, region, and registry URI
