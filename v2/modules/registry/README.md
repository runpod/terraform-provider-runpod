# Registry credentials and ECR delegation

Stores container registry credentials for pulling private images and,
when `ecr_resource_arn` is given, delegates pulls for an AWS ECR resource.
Both are free to keep. The credential `id` is the `registry` input of the
pod, endpoint and template modules; the delegation's `docker_registry_uri`
is the registry images then resolve through.

Both need real external input, passed as variables rather than hardcoded:

```bash
terraform apply -var username=... -var password=... \
  -var ecr_resource_arn=arn:aws:ecr:us-east-1:123456789012:repository/trainers
```

Neither resource supports `terraform import`: the API never returns the
credential secret or the delegated ARN, so an imported resource could not be
reconciled against configuration. The listings read back every credential
and delegation on the account. The `registry_usernames` output exists for
1.x configurations and is always null: no supported API reports a
credential's username.

Build and run it with the shared flow in [../README.md](../README.md).

## Usage

```hcl
module "registry" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/registry?ref=v2.0.0-rc.3"

  name     = "ghcr"
  username = var.registry_username
  password = var.registry_password
}
```

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| terraform | >= 1.6 |
| runpod | 2.0.0-rc.3 |

## Providers

| Name | Version |
| ---- | ------- |
| runpod | 2.0.0-rc.3 |

## Resources

| Name | Type |
| ---- | ---- |
| [runpod_registry.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/resources/registry) | resource |
| [runpod_registry_delegation.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/resources/registry_delegation) | resource |
| [runpod_registries.all](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/data-sources/registries) | data source |
| [runpod_registry_delegations.all](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.3/docs/data-sources/registry_delegations) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| password | Registry password or access token. No default: pass a real secret, never hardcode one. | `string` | n/a | yes |
| username | Registry username. No default: pass a real login. | `string` | n/a | yes |
| delegation\_name | ECR delegation name. | `string` | `"example-delegation"` | no |
| ecr\_resource\_arn | AWS ECR resource ARN to delegate pulls for, e.g. arn:aws:ecr:us-east-1:123456789012:repository/trainers. Null creates no delegation. | `string` | `null` | no |
| name | Credential name. | `string` | `"example-registry"` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| delegation\_id | Identifier of the ECR delegation; null when no ARN was given. |
| delegation\_ids | Identifiers of every ECR delegation on the account. |
| delegations | Every ECR delegation on the account, with the AWS region and principal to authorize. |
| docker\_registry\_uri | Registry URI images resolve through once delegated; null when no ARN was given. |
| id | Credential identifier, used as the registry input of the pod, endpoint and template modules. |
| registry\_ids | Identifiers of every registry credential on the account, including this one. |
| registry\_usernames | Deprecated. Credential id to null for every credential on the account, kept for 1.x configurations: no supported API reports a registry username. |
<!-- END_TF_DOCS -->
