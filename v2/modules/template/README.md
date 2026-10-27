# Template

Creates a reusable pod / endpoint configuration template, reads it back by id
through the `runpod_template` data source, and lists every template visible
to the account. Configuration metadata only, so it is free to keep.

The template's `id` is what the [gpu-pod](../gpu-pod), [cpu-pod](../cpu-pod)
and Serverless endpoint modules take as `template_id`; `serverless = true`
marks a template for endpoints. A private image needs a `registry`
credential (see [registry](../registry)). `persistent_volume` gives every
container created from the template a host-local mount.

`docker_entrypoint` is the v2 template's entrypoint. The API stores it inside
`args`, so the two cannot both be set; to set an entrypoint and a command
together, write `args` in its JSON form.

The deprecated inputs and outputs (`readme`, `earned`, `is_runpod`,
`runtime_in_min`) go through the GraphQL compatibility shim and exist for 1.x
configurations. `readme` cannot be cleared once set: an empty value is
rejected at plan time.

Build and run it with the shared flow in [../README.md](../README.md), then
`terraform destroy`.

## Usage

```hcl
module "template" {
  source = "github.com/runpod/terraform-provider-runpod//v2/modules/template?ref=v2.0.0-rc.4"

  name  = "pytorch-base"
  image = "runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"
  disk  = 20
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
| [runpod_template.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.4/docs/resources/template) | resource |
| [runpod_template.this](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.4/docs/data-sources/template) | data source |
| [runpod_templates.all](https://registry.terraform.io/providers/runpod/runpod/2.0.0-rc.4/docs/data-sources/templates) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| allowed\_cuda\_versions | Exact set of acceptable host CUDA versions for containers created from this template, e.g. ["12.8"]. Null accepts any version. | `list(string)` | `null` | no |
| args | Arguments passed to the container entrypoint. Null keeps the image's own. | `string` | `null` | no |
| category | Console grouping of a public template: CPU, NVIDIA, or AMD. Null leaves it unset. | `string` | `null` | no |
| disk | Container disk in GB (ephemeral). | `number` | `20` | no |
| docker\_entrypoint | Container entrypoint for containers created from this template, in exec form, e.g. ["/bin/bash"], preserving the 1.x docker\_entrypoint attribute. Null leaves the template's own value in place; [] clears it. Cannot be set together with args. | `list(string)` | `null` | no |
| env | Environment variables. | `map(string)` | <pre>{<br/>  "EXAMPLE": "true"<br/>}</pre> | no |
| image | Docker image reference. | `string` | `"runpod/pytorch:2.8.0-py3.11-cuda12.8.1-cudnn-devel-ubuntu22.04"` | no |
| name | Template name. | `string` | `"example-template"` | no |
| persistent\_volume | Host-local persistent storage for containers created from this template, replacing the 1.x volume\_in\_gb and volume\_mount\_path attributes. Null attaches none. | <pre>object({<br/>    size = number<br/>    path = string<br/>  })</pre> | `null` | no |
| ports | Exposed ports. | `list(string)` | <pre>[<br/>  "8888/http",<br/>  "22/tcp"<br/>]</pre> | no |
| public | Whether the template is shared publicly in the catalog. Null uses the API default, which is private. | `bool` | `null` | no |
| readme | Deprecated. Markdown description shown on the template's console page, preserving the 1.x readme attribute. Null leaves the template's own text in place. The text cannot be cleared: an empty or whitespace-only value is rejected at plan time. | `string` | `null` | no |
| registry | Registry credential ID (see the registry module) for pulling private images, called container\_registry\_auth\_id in 1.x. Null for public images. | `string` | `null` | no |
| serverless | Whether the template configures Serverless workers (true) or pods (false). Null uses the API default, which is pods. | `bool` | `null` | no |
| start\_jupyter | Start JupyterLab in containers created from this template. Null uses the API default, which is true. | `bool` | `null` | no |
| start\_ssh | Provision SSH access in containers created from this template. Null uses the API default, which is true. | `bool` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| allowed\_cuda\_versions | Effective CUDA version constraint; empty means any version. |
| docker\_entrypoint | Container entrypoint containers from this template run. |
| earned | Deprecated. Earnings in USD credited to this template through the creator programme; zero for a private template. |
| id | Template identifier, usable as template\_id in the pod and endpoint modules. |
| image | Image the template deploys. |
| is\_runpod | Deprecated. Whether Runpod itself publishes the template. |
| lookup\_docker\_entrypoint | Container entrypoint as the runpod\_template data source reads it back from the v2 template. |
| lookup\_image | Image as the runpod\_template data source reads it back by id. |
| lookup\_start\_ssh | SSH flag as the runpod\_template data source reads it back by id. |
| persistent\_volume | Persistent mount containers from this template receive, if any. |
| readme | Deprecated. Markdown description shown on the template's console page. |
| runtime\_in\_min | Deprecated. Total minutes containers created from this template have run. |
| serverless | Whether the template is serverless-compatible (API-derived). |
| start\_jupyter | Whether containers from this template start JupyterLab (API default when not configured). |
| start\_ssh | Whether containers from this template provision SSH access (API default when not configured). |
| template\_ids | Identifiers of every template visible to the account, including this one. |
<!-- END_TF_DOCS -->
