---
page_title: "Account SSH keys"
subcategory: "Use-case modules"
description: |-
  Registers the SSH public keys that pods and cluster nodes created with SSH enabled accept.
---

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

## Usage

```hcl
module "account_ssh_keys" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/account-ssh-keys?ref=v2.0.0-rc.3"

  keys              = [trimspace(file("~/.ssh/id_ed25519.pub"))]
  remove_on_destroy = false
}
```

## Resources and data sources

- [`runpod_account_ssh_keys`](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/account_ssh_keys)

## Configuration

```terraform
resource "runpod_account_ssh_keys" "this" {
  keys              = var.keys
  remove_on_destroy = var.remove_on_destroy
}
```

Inputs and outputs are documented in the
[module README](https://github.com/runpod/terraform-provider-runpod/blob/v2.0.0-rc.3/v2/modules/account-ssh-keys/README.md).
