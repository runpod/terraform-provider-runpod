# Runpod Terraform Provider

Manage Runpod GPU and CPU pods, Serverless endpoints, templates, network volumes, Instant Clusters,
account SSH keys and container registry credentials with Terraform, through the Runpod REST API v2.

<!-- runpod-agent-setup:start -->
**Using an AI agent?** Point it to https://agents.runpod.io to install the Runpod skills and MCP server.
<!-- runpod-agent-setup:end -->

```hcl
terraform {
  required_version = ">= 1.6"

  required_providers {
    runpod = {
      source  = "runpod/runpod"
      version = "2.0.0-rc.3"
    }
  }
}

provider "runpod" {} # reads RUNPOD_API_KEY
```

- Documentation: https://registry.terraform.io/providers/runpod/runpod/latest/docs
- Upgrading from 1.x: 2.0.0 is a breaking release; read the
  [migration guide](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/migrating-from-1-x) first.
- Using the provider from an AI agent: [agent guide](https://registry.terraform.io/providers/runpod/runpod/latest/docs/guides/agent-setup)
- Runnable use-case modules: [v2/modules/](v2/modules/)
- Source and development: [v2/README.md](v2/README.md)
- Release notes: [CHANGELOG.md](CHANGELOG.md)
- License: MIT, see [v2/LICENSE](v2/LICENSE)

The 1.x provider (1.0.x tags) is built from the Go module at the repository root; its files stay here
only for 1.0.x patch releases.
Root `*.md` files other than README.md and CHANGELOG.md describe the 1.x provider, refer to a 1.x
`examples/` directory that is no longer in the tree, and do not work with 2.x.
