package provider

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Conversions between the generated API types and framework values, plus the
// mount shapes every pod-flavoured resource and data source maps onto. Pods,
// pod data sources, and clusters all model the same two API objects
// (PersistentMount and NetworkMount) with the same tfsdk tags, so they share
// one pair here instead of keeping a copy each.

type persistentMountModel struct {
	Path types.String `tfsdk:"path"`
	Size types.Int64  `tfsdk:"size"`
}

type networkMountModel struct {
	Path     types.String `tfsdk:"path"`
	VolumeId types.String `tfsdk:"volume_id"`
}

var persistentMountAttrTypes = map[string]attr.Type{
	"path": types.StringType,
	"size": types.Int64Type,
}

var networkMountAttrTypes = map[string]attr.Type{
	"path":      types.StringType,
	"volume_id": types.StringType,
}

var networkMountObjectType = types.ObjectType{AttrTypes: networkMountAttrTypes}

// int64PointerValue converts an optional API int to an Int64 value, since the
// framework's Int64PointerValue takes *int64 rather than the *int the
// generated client uses.
func int64PointerValue(value *int) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*value))
}

// widenFloat32 converts a float32 the API round-tripped back to the float64
// the framework holds, through the shortest decimal that still reads back as
// the same float32. Widening directly would turn a planned 0.7 into
// 0.699999988079071 and fail the plan-versus-state comparison.
func widenFloat32(value float32) (float64, error) {
	return strconv.ParseFloat(strconv.FormatFloat(float64(value), 'f', -1, 32), 64)
}

// isEmptyStringValue reports whether a string attribute holds a known empty
// string, which some API fields treat as "clear this" and answer with null.
func isEmptyStringValue(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueString() == ""
}

// isEmptyListValue reports whether a list attribute holds a known empty list,
// which some API fields omit from the response rather than echo back.
func isEmptyListValue(value types.List) bool {
	return !value.IsNull() && !value.IsUnknown() && len(value.Elements()) == 0
}
