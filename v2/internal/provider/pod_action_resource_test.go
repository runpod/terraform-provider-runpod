package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

func TestExpandPodAction(t *testing.T) {
	for _, action := range []sdktypes.PodAction{
		sdktypes.PodActionStart,
		sdktypes.PodActionStop,
		sdktypes.PodActionRestart,
		sdktypes.PodActionTerminate,
	} {
		body := expandPodAction(podActionResourceModel{
			PodId:  types.StringValue("pod_abc123"),
			Action: types.StringValue(string(action)),
		})
		if body.Action != action {
			t.Errorf("action = %q, want %q", body.Action, action)
		}
		if !body.Action.Valid() {
			t.Errorf("action %q is not a member of the API enum", body.Action)
		}
	}
}

func TestPodActionSchemaValidatesAction(t *testing.T) {
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&podActionResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	attribute, ok := schemaResp.Schema.Attributes["action"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("action attribute is %T, want a StringAttribute", schemaResp.Schema.Attributes["action"])
	}

	for value, wantValid := range map[string]bool{
		"start":     true,
		"stop":      true,
		"restart":   true,
		"terminate": true,
		"START":     false,
		"pause":     false,
		"":          false,
	} {
		var valResp validator.StringResponse
		for _, v := range attribute.Validators {
			v.ValidateString(ctx, validator.StringRequest{
				Path:        path.Root("action"),
				ConfigValue: types.StringValue(value),
			}, &valResp)
		}
		if valResp.Diagnostics.HasError() == wantValid {
			t.Errorf("action %q: valid = %v, want %v", value, !valResp.Diagnostics.HasError(), wantValid)
		}
	}
}

func TestPodActionDeleteCallsNoAPI(t *testing.T) {
	// The resource is built with no client at all: if Delete reached the API it
	// would panic on the nil client instead of returning clean.
	r := &podActionResource{}
	var resp resource.DeleteResponse
	r.Delete(context.Background(), resource.DeleteRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("destroying a pod action must not call the API: %v", resp.Diagnostics)
	}
}

func TestPodActionSchemaReplacesOnPodId(t *testing.T) {
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&podActionResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	podId, ok := schemaResp.Schema.Attributes["pod_id"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("pod_id attribute is %T, want a StringAttribute", schemaResp.Schema.Attributes["pod_id"])
	}
	if len(podId.PlanModifiers) == 0 {
		t.Fatal("pod_id must carry RequiresReplace: retargeting the resource has to act on the new pod")
	}

	// action and triggers re-run the action in place, so neither may force a
	// replacement.
	action, _ := schemaResp.Schema.Attributes["action"].(resourceschema.StringAttribute)
	if len(action.PlanModifiers) != 0 {
		t.Fatal("action must not force a replacement; a change re-runs the action")
	}
	triggers, _ := schemaResp.Schema.Attributes["triggers"].(resourceschema.MapAttribute)
	if len(triggers.PlanModifiers) != 0 {
		t.Fatal("triggers must not force a replacement; a change re-runs the action")
	}
}

func TestFlattenPodActionStatus(t *testing.T) {
	// start, stop, and restart answer 200 with the updated pod, so the status
	// the 1.x resource read off the GraphQL mutation comes back from REST v2
	// without a follow-up read.
	status := flattenPodActionStatus(&sdktypes.Pod{Id: "pod_abc123", Status: sdktypes.PodStatusExited})
	if status.ValueString() != string(sdktypes.PodStatusExited) {
		t.Errorf("status = %v, want EXITED", status)
	}

	// terminate answers 204 with no body.
	if empty := flattenPodActionStatus(nil); !empty.IsNull() {
		t.Errorf("status = %v, want null when the action answered no content", empty)
	}
}

func TestPodActionStatusIsComputed(t *testing.T) {
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&podActionResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	status, ok := schemaResp.Schema.Attributes["status"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("status attribute is %T, want a StringAttribute", schemaResp.Schema.Attributes["status"])
	}
	if !status.Computed || status.Optional || status.Required {
		t.Errorf("status must be computed only: %+v", status)
	}
}
