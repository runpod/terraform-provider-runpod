# Pod lifecycle

Runs one state transition (start, stop, restart or terminate) on a pod
that already exists, then reads the pod back and lists every pod on the
account. Use it to drive a pod created outside Terraform, or a pod from
[gpu-pod](../gpu-pod) / [cpu-pod](../cpu-pod) whose run state you do not
want managed declaratively.

```bash
terraform apply -var pod_id=gfj8b292vyg08g -var action=restart
```

`triggers` is the usual way to tie a restart to something else moving:

```hcl
triggers = {
  image = var.image
}
```

Free by itself, but the transition it runs changes what the pod costs:
`start` resumes hourly billing, `stop` releases the compute, `terminate`
deletes the pod. Do not point this at a pod whose `runpod_pod` resource sets
`desired_state`: that attribute manages the same transitions declaratively,
and the two would undo each other on every apply.

The `status` output comes from the deprecated GraphQL shim and exists for
1.x configurations; `pod_status` is the supported read.

Build and run it with the shared flow in [../README.md](../README.md).

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| terraform | >= 1.6 |
| runpod | ~> 2.0 |

## Providers

| Name | Version |
| ---- | ------- |
| runpod | ~> 2.0 |

## Resources

| Name | Type |
| ---- | ---- |
| [runpod_pod_action.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/resources/pod_action) | resource |
| [runpod_pod.this](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/pod) | data source |
| [runpod_pods.all](https://registry.terraform.io/providers/runpod/runpod/latest/docs/data-sources/pods) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| pod\_id | Pod to act on. No default: the action runs against a real pod as soon as you apply. | `string` | n/a | yes |
| action | Transition to run: start, stop, restart, or terminate. | `string` | `"restart"` | no |
| include\_cluster\_pods | Include pods that belong to a cluster in the account listing. Null uses the API default, which leaves them out. | `bool` | `null` | no |
| triggers | Values that re-run the action when any of them changes, e.g. { image = var.image }. | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| action | The transition that ran. |
| id | The pod the action ran against. |
| pod\_cost\_per\_hour | What the pod costs per hour now, in USD; zero once stopped. |
| pod\_gpu\_type\_id | GPU type the pod was placed on; null for a CPU pod. |
| pod\_ids | Identifiers of every pod on the account. |
| pod\_ssh\_command | Proxy SSH command for the pod; null until it has a machine assignment. |
| pod\_status | Pod lifecycle status read back after the transition. |
| pod\_uptime\_seconds | Seconds since the pod's container started; null unless the pod is RUNNING. |
| status | Deprecated. Pod status as the action's GraphQL shim reported it right after the transition; null when that host does not answer. |
<!-- END_TF_DOCS -->
