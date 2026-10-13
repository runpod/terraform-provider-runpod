package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The pod's live utilization block, shared by the runpod_pod resource and the
// runpod_pod data source so both report the same shape. Object-valued rather
// than struct-pointer models because the resource reads its model out of a
// plan where a Computed block is unknown, which a struct pointer cannot hold.

type podRuntimeModel struct {
	UptimeSeconds types.Int64 `tfsdk:"uptime_seconds"`
	CpuUtil       types.Int64 `tfsdk:"cpu_util"`
	MemoryUtil    types.Int64 `tfsdk:"memory_util"`
	Gpus          types.List  `tfsdk:"gpus"`
	Ports         types.List  `tfsdk:"ports"`
}

type podGpuUtilizationModel struct {
	Util       types.Int64 `tfsdk:"util"`
	MemoryUtil types.Int64 `tfsdk:"memory_util"`
}

type podRuntimePortModel struct {
	Ip          types.String `tfsdk:"ip"`
	PrivatePort types.Int64  `tfsdk:"private_port"`
	PublicPort  types.Int64  `tfsdk:"public_port"`
	Type        types.String `tfsdk:"type"`
}

var podGpuUtilizationAttrTypes = map[string]attr.Type{
	"util":        types.Int64Type,
	"memory_util": types.Int64Type,
}

var podRuntimePortAttrTypes = map[string]attr.Type{
	"ip":           types.StringType,
	"private_port": types.Int64Type,
	"public_port":  types.Int64Type,
	"type":         types.StringType,
}

var (
	podGpuUtilizationObjectType = types.ObjectType{AttrTypes: podGpuUtilizationAttrTypes}
	podRuntimePortObjectType    = types.ObjectType{AttrTypes: podRuntimePortAttrTypes}
)

var podRuntimeAttrTypes = map[string]attr.Type{
	"uptime_seconds": types.Int64Type,
	"cpu_util":       types.Int64Type,
	"memory_util":    types.Int64Type,
	"gpus":           types.ListType{ElemType: podGpuUtilizationObjectType},
	"ports":          types.ListType{ElemType: podRuntimePortObjectType},
}

// Plan modifiers for blocks whose presence, not content, is fixed at create:
// a pod's mount kind and an endpoint's compute family. The mount shapes
// themselves live in convert.go, shared with the pod data source and clusters.

// requiresReplaceOnObjectAdded replaces the resource when a nested block is
// added, while leaving an edit inside an existing block as an in-place update.
// It is for the pod's mounts, which the API cannot detach: a block dropped
// from the configuration, or one a template supplied, is not a removal. The
// mounts are Optional+Computed, so the framework plans them unknown on any
// other change; UseStateForUnknown has to run first to keep them.
func requiresReplaceOnObjectAdded(description string) planmodifier.Object {
	return objectplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.ObjectRequest, resp *objectplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = req.StateValue.IsNull() && !req.ConfigValue.IsNull()
		},
		description,
		description,
	)
}

// requiresReplaceOnListAdded is the list counterpart of
// requiresReplaceOnObjectAdded.
func requiresReplaceOnListAdded(description string) planmodifier.List {
	return listplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.ListRequest, resp *listplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = req.StateValue.IsNull() && !req.ConfigValue.IsNull()
		},
		description,
		description,
	)
}

// requiresReplaceOnListPresenceChange replaces the resource when a nested
// block appears or disappears, while leaving an edit inside an existing block
// as an in-place update: an endpoint's compute family, where dropping one
// block means switching to the other.
func requiresReplaceOnListPresenceChange(description string) planmodifier.List {
	return listplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.ListRequest, resp *listplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = req.StateValue.IsNull() != req.ConfigValue.IsNull()
		},
		description,
		description,
	)
}
