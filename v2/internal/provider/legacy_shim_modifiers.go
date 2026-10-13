package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Plan modifiers for the REST v1 shim read-backs.
//
// Every one of them is Computed, and terraform core copies the prior state
// value into the proposed plan for a computed attribute the configuration does
// not set (objchange.ProposedNew), which the framework then leaves known. The
// shim legitimately answers with something else (the platform increments an
// endpoint's version on every change, workers are a live snapshot), and a
// legacy host that has gone away nulls all of them, so an apply that promised
// the previous value fails with "Provider produced inconsistent result after
// apply". Planning them unknown while the resource is being changed lets
// whatever comes back land, including the null the warn-and-null read path
// produces.
//
// Two cases deliberately keep their planned value: a plan with no changes at
// all, because turning a no-op into an update would put a permanent diff on
// every plan, and an attribute the configuration sets, because there the
// configuration decides the value rather than the shim.

const legacyShimModifierDescription = "Planned as unknown while the resource changes, because the deprecated " +
	"REST v1 shim reports a live value and answers null when the legacy host is gone."

// legacyShimPlansUnknown is the shared rule; the per-type modifiers below
// differ only in the unknown they write.
func legacyShimPlansUnknown(state, plan tftypes.Value, config attr.Value) bool {
	// No prior state is a create, no plan is a destroy, and a plan equal to
	// the state is a no-op that must stay one.
	if state.IsNull() || plan.IsNull() || plan.Equal(state) {
		return false
	}
	return config.IsNull()
}

// legacyShimModifier carries the description both halves of every
// planmodifier interface need.
type legacyShimModifier struct{}

func (legacyShimModifier) Description(context.Context) string { return legacyShimModifierDescription }

func (legacyShimModifier) MarkdownDescription(context.Context) string {
	return legacyShimModifierDescription
}

func legacyShimUnknownString() planmodifier.String { return legacyShimStringModifier{} }

type legacyShimStringModifier struct{ legacyShimModifier }

func (legacyShimStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if legacyShimPlansUnknown(req.State.Raw, req.Plan.Raw, req.ConfigValue) {
		resp.PlanValue = types.StringUnknown()
	}
}

func legacyShimUnknownBool() planmodifier.Bool { return legacyShimBoolModifier{} }

type legacyShimBoolModifier struct{ legacyShimModifier }

func (legacyShimBoolModifier) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if legacyShimPlansUnknown(req.State.Raw, req.Plan.Raw, req.ConfigValue) {
		resp.PlanValue = types.BoolUnknown()
	}
}

func legacyShimUnknownInt64() planmodifier.Int64 { return legacyShimInt64Modifier{} }

type legacyShimInt64Modifier struct{ legacyShimModifier }

func (legacyShimInt64Modifier) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if legacyShimPlansUnknown(req.State.Raw, req.Plan.Raw, req.ConfigValue) {
		resp.PlanValue = types.Int64Unknown()
	}
}

func legacyShimUnknownFloat64() planmodifier.Float64 { return legacyShimFloat64Modifier{} }

type legacyShimFloat64Modifier struct{ legacyShimModifier }

func (legacyShimFloat64Modifier) PlanModifyFloat64(_ context.Context, req planmodifier.Float64Request, resp *planmodifier.Float64Response) {
	if legacyShimPlansUnknown(req.State.Raw, req.Plan.Raw, req.ConfigValue) {
		resp.PlanValue = types.Float64Unknown()
	}
}

// legacyShimUnknownList takes the element type rather than reading it off the
// planned value, so the unknown it writes carries the schema's type whatever
// state that value is in.
func legacyShimUnknownList(elementType attr.Type) planmodifier.List {
	return legacyShimListModifier{elementType: elementType}
}

type legacyShimListModifier struct {
	legacyShimModifier
	elementType attr.Type
}

func (m legacyShimListModifier) PlanModifyList(_ context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if legacyShimPlansUnknown(req.State.Raw, req.Plan.Raw, req.ConfigValue) {
		resp.PlanValue = types.ListUnknown(m.elementType)
	}
}
