package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The shim read-backs are Computed, so terraform core proposes the prior state
// value for them on an update and the apply then has to produce exactly that,
// which neither a live value (version, workers) nor the null a dead legacy
// host produces can promise. These pin the four cases the modifier has to tell
// apart.

var shimModifierObjectType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String}}

func shimModifierValue(name string) tftypes.Value {
	return tftypes.NewValue(shimModifierObjectType, map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, name),
	})
}

func TestLegacyShimModifierPlansUnknownOnlyOnAChangingUnconfiguredUpdate(t *testing.T) {
	ctx := context.Background()
	nullObject := tftypes.NewValue(shimModifierObjectType, nil)
	before := shimModifierValue("trainer")
	after := shimModifierValue("renamed")

	for _, tc := range []struct {
		name        string
		state, plan tftypes.Value
		config      types.String
		wantUnknown bool
	}{
		{
			name:  "create keeps the planned value",
			state: nullObject, plan: after, config: types.StringNull(),
		},
		{
			name:  "destroy keeps the planned value",
			state: before, plan: nullObject, config: types.StringNull(),
		},
		{
			name:  "a plan with no changes stays a no-op",
			state: before, plan: before, config: types.StringNull(),
		},
		{
			name:  "a configured value belongs to the configuration",
			state: before, plan: after, config: types.StringValue("# readme"),
		},
		{
			name:  "an update of an unconfigured read-back plans unknown",
			state: before, plan: after, config: types.StringNull(),
			wantUnknown: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := planmodifier.StringResponse{PlanValue: types.StringValue("mach-1")}
			legacyShimUnknownString().PlanModifyString(ctx, planmodifier.StringRequest{
				State:       tfsdk.State{Raw: tc.state},
				Plan:        tfsdk.Plan{Raw: tc.plan},
				ConfigValue: tc.config,
			}, &resp)

			if resp.PlanValue.IsUnknown() != tc.wantUnknown {
				t.Fatalf("planned %v, want unknown = %v", resp.PlanValue, tc.wantUnknown)
			}
			if !tc.wantUnknown && resp.PlanValue.ValueString() != "mach-1" {
				t.Fatalf("planned %v, want the incoming value kept", resp.PlanValue)
			}
		})
	}
}

func TestLegacyShimModifiersCoverEveryShimValueType(t *testing.T) {
	ctx := context.Background()
	state := tfsdk.State{Raw: shimModifierValue("trainer")}
	plan := tfsdk.Plan{Raw: shimModifierValue("renamed")}

	boolResp := planmodifier.BoolResponse{PlanValue: types.BoolValue(true)}
	legacyShimUnknownBool().PlanModifyBool(ctx, planmodifier.BoolRequest{
		State: state, Plan: plan, ConfigValue: types.BoolNull(),
	}, &boolResp)
	if !boolResp.PlanValue.IsUnknown() {
		t.Errorf("bool planned %v, want unknown", boolResp.PlanValue)
	}

	int64Resp := planmodifier.Int64Response{PlanValue: types.Int64Value(3)}
	legacyShimUnknownInt64().PlanModifyInt64(ctx, planmodifier.Int64Request{
		State: state, Plan: plan, ConfigValue: types.Int64Null(),
	}, &int64Resp)
	if !int64Resp.PlanValue.IsUnknown() {
		t.Errorf("int64 planned %v, want unknown", int64Resp.PlanValue)
	}

	float64Resp := planmodifier.Float64Response{PlanValue: types.Float64Value(1.25)}
	legacyShimUnknownFloat64().PlanModifyFloat64(ctx, planmodifier.Float64Request{
		State: state, Plan: plan, ConfigValue: types.Float64Null(),
	}, &float64Resp)
	if !float64Resp.PlanValue.IsUnknown() {
		t.Errorf("float64 planned %v, want unknown", float64Resp.PlanValue)
	}

	listResp := planmodifier.ListResponse{PlanValue: types.ListNull(types.StringType)}
	legacyShimUnknownList(types.StringType).PlanModifyList(ctx, planmodifier.ListRequest{
		State: state, Plan: plan, ConfigValue: types.ListNull(types.StringType),
	}, &listResp)
	if !listResp.PlanValue.IsUnknown() {
		t.Errorf("list planned %v, want unknown", listResp.PlanValue)
	}
	if listResp.PlanValue.ElementType(ctx) != types.StringType {
		t.Errorf("list element type = %v, want the schema's", listResp.PlanValue.ElementType(ctx))
	}
}

func TestEveryShimReadBackCarriesThePlanModifier(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		resource   resource.Resource
		attributes []string
	}{
		{&podResource{}, []string{"machine_id", "volume_encrypted"}},
		{&serverlessEndpointResource{}, []string{"template_version", "users", "version", "workers"}},
		{&templateResource{}, []string{"docker_entrypoint", "readme", "earned", "is_runpod", "runtime_in_min"}},
	} {
		var metaResp resource.MetadataResponse
		tc.resource.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "runpod"}, &metaResp)

		var schemaResp resource.SchemaResponse
		tc.resource.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
		if schemaResp.Diagnostics.HasError() {
			t.Fatal(schemaResp.Diagnostics)
		}

		for _, name := range tc.attributes {
			attribute, ok := schemaResp.Schema.Attributes[name]
			if !ok {
				t.Errorf("%s has no %q attribute", metaResp.TypeName, name)
				continue
			}
			if shimPlanModifierCount(attribute) == 0 {
				t.Errorf("%s.%s has no plan modifier, so an update plans it known at its previous value",
					metaResp.TypeName, name)
			}
		}
	}
}

// shimPlanModifierCount reports how many plan modifiers an attribute carries,
// across the attribute kinds the shim read-backs use.
func shimPlanModifierCount(attribute resourceschema.Attribute) int {
	switch typed := attribute.(type) {
	case resourceschema.StringAttribute:
		return len(typed.PlanModifiers)
	case resourceschema.BoolAttribute:
		return len(typed.PlanModifiers)
	case resourceschema.Int64Attribute:
		return len(typed.PlanModifiers)
	case resourceschema.Float64Attribute:
		return len(typed.PlanModifiers)
	case resourceschema.ListAttribute:
		return len(typed.PlanModifiers)
	case resourceschema.ListNestedAttribute:
		return len(typed.PlanModifiers)
	default:
		return 0
	}
}
