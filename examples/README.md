# RunPod Terraform Provider Demo

This directory contains demo configurations for the RunPod Terraform provider.

## Prerequisites

1. Get your RunPod API key from https://runpod.io/console/user/settings
2. Update `variables.tf` files with your API key and machine IDs
3. Set up development environment: see `LOCAL_SETUP.md` for recommended approach

## Available Demos

### 1. Basic Pod Creation (`examples/basic/`)

Creates a simple pod with SSH and Jupyter enabled.

```bash
cd examples/basic
terraform init
terraform apply
```

### 2. Pod Actions (`examples/actions/`)

Demonstrates stopping, resuming, terminating, and resetting pods.

```bash
cd examples/actions
terraform init
terraform apply -var="action=stop"
```

### 3. Data Sources (`examples/datasources/`)

Queries available GPU types, data centers, machines, and user info.

```bash
cd examples/datasources
terraform init
terraform plan
```

### 4. Machine Bidding (`examples/machine/`)

Lists machines for others to use with custom specifications.

```bash
cd examples/machine
terraform init
terraform apply
```

### 5. Pod Monitoring (`examples/monitoring/`)

Monitors pod status and retrieves detailed information.

```bash
cd examples/monitoring
terraform init
terraform plan
```

## Example Workflow

1. **List available GPU types:**
    ```bash
    cd examples/datasources
    terraform init
    terraform apply -var="runpod_api_key=your-key"
    ```

2. **Create a pod:**
    ```bash
    cd examples/basic
    terraform init
    terraform apply -var="runpod_api_key=your-key" -var="machine_id=your-machine-id"
    ```

3. **Monitor the pod:**
    ```bash
    cd examples/monitoring
    terraform init
    terraform apply -var="runpod_api_key=your-key" -var="pod_id=created-pod-id"
    ```

4. **Stop the pod when done:**
    ```bash
    cd examples/actions
    terraform init
    terraform apply -var="runpod_api_key=your-key" -var="pod_id=created-pod-id" -var="action=stop"
    ```

## Notes

- Replace placeholder values in `variables.tf` with your actual credentials
- Pod creation may take several minutes
- Machines need to be listed before you can deploy pods to them
- Use data sources to discover available machines and GPU types
- For development, use `dev_overrides` (see `LOCAL_SETUP.md`) instead of building a binary
