# Network Volume Example

This example demonstrates how to create and manage standalone network volumes with Runpod.

## What's a Network Volume?

Runpod **network volumes** are persistent, portable storage resources that:
- Exist independently of any pod
- Can be attached to multiple pods
- Retain data even after pods are deleted
- Are stored on Runpod's network-attached storage infrastructure
- Support two tiers: STANDARD and HIGH_PERFORMANCE

## Pricing

- **Standard storage**: $0.07/GB/month (first 1 TB)
- **High-performance storage**: Premium tier with up to 3x throughput

## Configuration

The example shows:
- `name`: Volume name
- `size`: Size in GB (1-4000)
- `data_center_id`: Where the volume is hosted (e.g., US-MD-1, EU-RO-1)
- `storage_tier`: STANDARD or HIGH_PERFORMANCE

## Usage

```bash
cd examples/network_volume

# Set your API key
export RUNPOD_API_KEY="your-api-key-here"

# Create the network volume
terraform apply

# View volume details
terraform output volume_info

# Clean up (this will delete the network volume)
terraform destroy
```

## Notes

- Network volumes can be attached to pods during creation
- Volume size can only be increased, not decreased
- Data does not sync automatically between volumes in different datacenters
