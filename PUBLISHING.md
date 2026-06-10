# RunPod Terraform Provider Repository Setup

## What's Included
- Complete Terraform provider code for RunPod API
- All resource and data source implementations
- Example configurations
- Provider specification (OpenAPI-based)

## Publishing to Terraform Registry

To publish this provider to the Terraform Registry:

### 1. Prepare Your Repository
- Push your code to GitHub (see GitHub documentation for setup)
- Ensure your repository has proper tags and release structure

### 2. Publish via Terraform Registry
Follow the [Terraform Provider Publishing Guide](https://developer.hashicorp.com/terraform/registry/publishing):

```bash
# Tag a release
git tag v1.0.0
git push --tags

# Then publish via Terraform CLI or registry web UI
```

### 3. Build Binaries for All Platforms
To create releases with binaries for Windows, Linux, and macOS:

```bash
# Darwin ARM64
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o terraform-provider-runpod_darwin_arm64

# Linux AMD64
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o terraform-provider-runpod_linux_amd64

# Linux ARM64  
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -o terraform-provider-runpod_linux_arm64

# Windows AMD64
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build -o terraform-provider-runpod_windows_amd64.exe
```

## Provider Configuration
Users can configure the provider via:
1. Environment variable: `RUNPOD_API_KEY`
2. Provider block in Terraform config

## API Documentation
- REST API: https://rest.runpod.io/v1/docs
- GraphQL API: https://api.runpod.io/graphql
- Docs: https://docs.runpod.io
