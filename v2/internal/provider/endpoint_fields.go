package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Nested-attribute shape for a serverless endpoint's CPU worker selection.
// The API accepts several configurations per endpoint and places workers on
// whichever one has capacity.

type endpointCpuModel struct {
	CpuTypeId types.String `tfsdk:"cpu_type_id"`
	VcpuCount types.Int64  `tfsdk:"vcpu_count"`
}

var endpointCpuAttrTypes = map[string]attr.Type{
	"cpu_type_id": types.StringType,
	"vcpu_count":  types.Int64Type,
}

var endpointCpuObjectType = types.ObjectType{AttrTypes: endpointCpuAttrTypes}

// The endpoint's invocation URLs. The API returns one of two shapes depending
// on the endpoint's routing type (the queue operations, or the load
// balancer's base and health URLs), so this model is the union of both and the
// fields that do not apply to an endpoint come back null.

type endpointRequestUrlsModel struct {
	Base       types.String `tfsdk:"base"`
	Cancel     types.String `tfsdk:"cancel"`
	Health     types.String `tfsdk:"health"`
	PurgeQueue types.String `tfsdk:"purge_queue"`
	Retry      types.String `tfsdk:"retry"`
	Run        types.String `tfsdk:"run"`
	RunSync    types.String `tfsdk:"run_sync"`
	Status     types.String `tfsdk:"status"`
	Stream     types.String `tfsdk:"stream"`
}

var endpointRequestUrlsAttrTypes = map[string]attr.Type{
	"base":        types.StringType,
	"cancel":      types.StringType,
	"health":      types.StringType,
	"purge_queue": types.StringType,
	"retry":       types.StringType,
	"run":         types.StringType,
	"run_sync":    types.StringType,
	"status":      types.StringType,
	"stream":      types.StringType,
}
