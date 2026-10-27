# Account SSH keys

Registers the SSH public keys that pods and cluster nodes created with SSH
enabled accept. Free: it stores configuration and nothing runs.

The API keeps the keys as one document that every write replaces in full, so
this resource owns **all** of them: a key added in the console is removed on
the next apply, and `keys` has no default so an empty apply cannot unregister
what you already have.

```bash
terraform apply -var 'keys=["ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... me@example.com"]'
```

Destroying leaves the keys registered and only forgets them, since losing them
would lock you out of every running pod. Pass `-var remove_on_destroy=true`
to have destroy clear them instead.

An account whose keys already exist is adopted rather than rewritten:

```bash
terraform import module.account_ssh_keys.runpod_account_ssh_keys.this account
```

Build and run it with the shared flow in [../README.md](../README.md).

## Usage

```hcl
module "account_ssh_keys" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/account-ssh-keys?ref=v2.0.0-rc.4"

  keys              = [trimspace(file("~/.ssh/id_ed25519.pub"))]
  remove_on_destroy = false
}
```

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| terraform | >= 1.6 |
| runpod | 2.0.0-rc.4 |

## Providers

| Name | Version |
| ---- | ------- |
| runpod | 2.0.0-rc.4 |

## Resources

| Name | Type |
| ---- | ---- |
| [runpod_account_ssh_keys.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.4/docs/resources/account_ssh_keys) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| keys | The complete set of SSH public keys to register on the account, one<br/>authorized\_keys-style line each. No default on purpose: every write<br/>replaces the whole set, so an accidental apply with an empty list would<br/>unregister the keys you already have. Pass your own, e.g.<br/>-var 'keys=["ssh-ed25519 AAAA... me@example.com"]'. | `set(string)` | n/a | yes |
| remove\_on\_destroy | Whether destroying the resource also clears the account's keys. False leaves them registered. | `bool` | `false` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| id | Always "account": the keys are one account-wide document. |
| key\_count | How many keys are registered. |
| keys | The keys now registered on the account, as the API reports them. |
<!-- END_TF_DOCS -->
