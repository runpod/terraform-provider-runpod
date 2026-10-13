package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The pod's mounts are Optional+Computed and fixed at create, and on any
// update the framework plans an unconfigured Computed attribute unknown. These
// run the schema's own modifier chain on that plan, the way the framework
// does, and pin that only adding a block replaces the pod: replacing it for a
// block the configuration leaves out would destroy the volume with it.

// A mount value per case: "" is null, "a" a mount, "b" the same mount edited.
type podMountCase struct {
	name                string
	state, config, plan string
	wantPlan            string
	wantReplace         bool
}

var podMountCases = []podMountCase{
	{
		name:  "a mount left out of the configuration is kept when another attribute changes",
		state: "a", config: "", plan: "unknown",
		wantPlan: "a",
	},
	{
		name:  "a mount left out of the configuration is kept when nothing else changes",
		state: "a", config: "", plan: "a",
		wantPlan: "a",
	},
	{
		name:  "adding a block replaces the pod",
		state: "", config: "a", plan: "a",
		wantPlan: "a", wantReplace: true,
	},
	{
		name:  "editing a block is applied in place",
		state: "a", config: "b", plan: "b",
		wantPlan: "b",
	},
	{
		name:  "a pod without a mount stays without one when another attribute changes",
		state: "", config: "", plan: "unknown",
		wantPlan: "",
	},
}

func persistentVolumeValue(kind string) types.Object {
	switch kind {
	case "":
		return types.ObjectNull(persistentMountAttrTypes)
	case "unknown":
		return types.ObjectUnknown(persistentMountAttrTypes)
	}
	size := map[string]int64{"a": 20, "b": 40}[kind]
	return types.ObjectValueMust(persistentMountAttrTypes, map[string]attr.Value{
		"size": types.Int64Value(size),
		"path": types.StringValue("/workspace"),
	})
}

func networkVolumesValue(kind string) types.List {
	switch kind {
	case "":
		return types.ListNull(networkMountObjectType)
	case "unknown":
		return types.ListUnknown(networkMountObjectType)
	}
	mountPath := map[string]string{"a": "/runpod-volume", "b": "/data"}[kind]
	return types.ListValueMust(networkMountObjectType, []attr.Value{
		types.ObjectValueMust(networkMountAttrTypes, map[string]attr.Value{
			"volume_id": types.StringValue("vol_abc"),
			"path":      types.StringValue(mountPath),
		}),
	})
}

func TestPodPersistentVolumeReplacesThePodOnlyWhenABlockIsAdded(t *testing.T) {
	ctx := context.Background()
	s, raw := shimResourceSchema(ctx, t, &podResource{})
	attribute, ok := s.Attributes["persistent_volume"].(resourceschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("persistent_volume = %T, want a single nested attribute", s.Attributes["persistent_volume"])
	}

	for _, tc := range podMountCases {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.ObjectRequest{
				State:       tfsdk.State{Schema: s, Raw: raw},
				Plan:        tfsdk.Plan{Schema: s, Raw: raw},
				StateValue:  persistentVolumeValue(tc.state),
				ConfigValue: persistentVolumeValue(tc.config),
				PlanValue:   persistentVolumeValue(tc.plan),
			}
			replace := false
			for _, modifier := range attribute.PlanModifiers {
				resp := planmodifier.ObjectResponse{PlanValue: req.PlanValue}
				modifier.PlanModifyObject(ctx, req, &resp)
				if resp.Diagnostics.HasError() {
					t.Fatal(resp.Diagnostics)
				}
				req.PlanValue = resp.PlanValue
				replace = replace || resp.RequiresReplace
			}

			if !req.PlanValue.Equal(persistentVolumeValue(tc.wantPlan)) {
				t.Errorf("planned %v, want %v", req.PlanValue, persistentVolumeValue(tc.wantPlan))
			}
			if replace != tc.wantReplace {
				t.Errorf("requires replace = %v, want %v", replace, tc.wantReplace)
			}
		})
	}
}

func TestPodNetworkVolumesReplaceThePodOnlyWhenABlockIsAdded(t *testing.T) {
	ctx := context.Background()
	s, raw := shimResourceSchema(ctx, t, &podResource{})
	attribute, ok := s.Attributes["network_volumes"].(resourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("network_volumes = %T, want a list nested attribute", s.Attributes["network_volumes"])
	}

	for _, tc := range podMountCases {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.ListRequest{
				State:       tfsdk.State{Schema: s, Raw: raw},
				Plan:        tfsdk.Plan{Schema: s, Raw: raw},
				StateValue:  networkVolumesValue(tc.state),
				ConfigValue: networkVolumesValue(tc.config),
				PlanValue:   networkVolumesValue(tc.plan),
			}
			replace := false
			for _, modifier := range attribute.PlanModifiers {
				resp := planmodifier.ListResponse{PlanValue: req.PlanValue}
				modifier.PlanModifyList(ctx, req, &resp)
				if resp.Diagnostics.HasError() {
					t.Fatal(resp.Diagnostics)
				}
				req.PlanValue = resp.PlanValue
				replace = replace || resp.RequiresReplace
			}

			if !req.PlanValue.Equal(networkVolumesValue(tc.wantPlan)) {
				t.Errorf("planned %v, want %v", req.PlanValue, networkVolumesValue(tc.wantPlan))
			}
			if replace != tc.wantReplace {
				t.Errorf("requires replace = %v, want %v", replace, tc.wantReplace)
			}
		})
	}
}
