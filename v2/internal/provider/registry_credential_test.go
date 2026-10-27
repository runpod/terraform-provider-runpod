package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// Every flatten copies registry from the API, and the API keeps a credential
// that a PATCH leaves out, or that a template supplied. A configuration without
// the attribute that planned null against an API still answering the
// credential would fail with "Provider produced inconsistent result after
// apply". These run each resource's own modifier chain and pin that the plan
// carries the value the flatten of that answer produces.

const keptRegistry = "reg_abc"

func TestRegistryPlanMatchesTheCredentialTheAPIKeeps(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name     string
		resource resource.Resource
		flatten  func(ctx context.Context) (types.String, diag.Diagnostics)
	}{
		{
			name:     "runpod_pod",
			resource: &podResource{},
			flatten: func(ctx context.Context) (types.String, diag.Diagnostics) {
				registry := keptRegistry
				var model podResourceModel
				diags := flattenPod(ctx, &sdktypes.Pod{
					Id:        "pod_abc",
					Name:      "trainer",
					Image:     "runpod/pytorch:2.8.0",
					Status:    sdktypes.PodStatusRunning,
					Cloud:     sdktypes.CloudSecure,
					Gpu:       &sdktypes.GpuConfig{Id: "NVIDIA GeForce RTX 4090"},
					CreatedAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
					Registry:  &registry,
				}, &model)
				return model.Registry, diags
			},
		},
		{
			name:     "runpod_template",
			resource: &templateResource{},
			flatten: func(ctx context.Context) (types.String, diag.Diagnostics) {
				registry := keptRegistry
				var model templateResourceModel
				diags := flattenTemplate(ctx, &sdktypes.Template{
					Id:       "tpl_abc",
					Name:     "trainer",
					Image:    "runpod/pytorch:2.8.0",
					Category: sdktypes.TemplateCategory("NVIDIA"),
					Disk:     30,
					Registry: &registry,
				}, &model)
				return model.Registry, diags
			},
		},
		{
			name:     "runpod_serverless_endpoint",
			resource: &serverlessEndpointResource{},
			flatten: func(ctx context.Context) (types.String, diag.Diagnostics) {
				registry := keptRegistry
				var scaling sdktypes.EndpointScaling
				if err := scaling.FromQueueDelayScaling(sdktypes.QueueDelayScaling{
					Type: sdktypes.QueueDelayScalingTypeQueueDelay, QueueDelay: 4,
				}); err != nil {
					t.Fatal(err)
				}
				var model serverlessEndpointResourceModel
				diags := flattenEndpoint(ctx, &sdktypes.Endpoint{
					Id:        "ep_abc",
					Name:      "worker",
					Timeout:   300000,
					Scaling:   scaling,
					Registry:  &registry,
					CreatedAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
				}, &model)
				return model.Registry, diags
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, raw := shimResourceSchema(ctx, t, tc.resource)
			attribute, ok := s.Attributes["registry"].(resourceschema.StringAttribute)
			if !ok {
				t.Fatalf("registry = %T, want a string attribute", s.Attributes["registry"])
			}
			if !attribute.Optional || !attribute.Computed {
				t.Fatalf("registry Optional = %v, Computed = %v, want both", attribute.Optional, attribute.Computed)
			}

			flattened, diags := tc.flatten(ctx)
			if diags.HasError() {
				t.Fatal(diags)
			}
			if flattened.ValueString() != keptRegistry {
				t.Fatalf("flattened registry = %v, want %q", flattened, keptRegistry)
			}

			noState := tftypes.NewValue(raw.Type(), nil)
			for _, step := range []struct {
				name                string
				stateRaw            tftypes.Value
				state, config, plan types.String
				want                types.String
			}{
				{
					name:     "dropped from the configuration while another attribute changes",
					stateRaw: raw,
					state:    types.StringValue(keptRegistry), config: types.StringNull(), plan: types.StringUnknown(),
					want: flattened,
				},
				{
					name:     "dropped from the configuration with nothing else changing",
					stateRaw: raw,
					state:    types.StringValue(keptRegistry), config: types.StringNull(), plan: types.StringValue(keptRegistry),
					want: flattened,
				},
				{
					name:     "changed in the configuration",
					stateRaw: raw,
					state:    types.StringValue(keptRegistry), config: types.StringValue("reg_new"), plan: types.StringValue("reg_new"),
					want: types.StringValue("reg_new"),
				},
				{
					name:     "unset on create, where a template can supply one",
					stateRaw: noState,
					state:    types.StringNull(), config: types.StringNull(), plan: types.StringUnknown(),
					want: types.StringUnknown(),
				},
			} {
				req := planmodifier.StringRequest{
					State:       tfsdk.State{Schema: s, Raw: step.stateRaw},
					Config:      tfsdk.Config{Schema: s, Raw: raw},
					Plan:        tfsdk.Plan{Schema: s, Raw: raw},
					StateValue:  step.state,
					ConfigValue: step.config,
					PlanValue:   step.plan,
				}
				if got := planRegistry(ctx, t, attribute, req); !got.Equal(step.want) {
					t.Errorf("%s: planned %v, want %v", step.name, got, step.want)
				}
			}
		})
	}
}

// template_id changes an endpoint in place, and a template can supply the
// credential. Every update that sends template_id re-applies the template, so
// even an unchanged id can bring a different credential once the template's
// own registry has changed: such an update must neither promise the old
// credential nor send it back as an explicit field, which would win over the
// template's.
func TestEndpointRegistryIsLeftToANewTemplate(t *testing.T) {
	ctx := context.Background()
	s, raw := shimResourceSchema(ctx, t, &serverlessEndpointResource{})
	attribute, ok := s.Attributes["registry"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("registry = %T, want a string attribute", s.Attributes["registry"])
	}

	state := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"template_id": types.StringValue("tpl_old"),
		"registry":    types.StringValue(keptRegistry),
	})
	for _, step := range []struct {
		name     string
		template types.String
		want     types.String
	}{
		{"the same template_id, whose credential may have changed", types.StringValue("tpl_old"), types.StringUnknown()},
		{"template_id left out, so none is sent", types.StringNull(), types.StringValue(keptRegistry)},
		{"a different template_id", types.StringValue("tpl_new"), types.StringUnknown()},
		{"a template_id not known until apply", types.StringUnknown(), types.StringUnknown()},
	} {
		req := planmodifier.StringRequest{
			State:       tfsdk.State{Schema: s, Raw: state},
			Config:      tfsdk.Config{Schema: s, Raw: shimObject(ctx, t, s, raw, map[string]attr.Value{"template_id": step.template})},
			Plan:        tfsdk.Plan{Schema: s, Raw: raw},
			StateValue:  types.StringValue(keptRegistry),
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
		}
		if got := planRegistry(ctx, t, attribute, req); !got.Equal(step.want) {
			t.Errorf("%s: planned %v, want %v", step.name, got, step.want)
		}
	}

	for _, tc := range []struct {
		name       string
		configured types.String
		want       any
	}{
		{"kept in the plan but not configured", types.StringNull(), nil},
		{"configured", types.StringValue("reg_new"), "reg_new"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sent map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPatch && r.URL.Path == "/v2/serverless/ep_abc":
					if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
						t.Error(err)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"id":"ep_abc","name":"renamed","flashboot":"OFF","createdAt":"2026-09-28T12:00:00Z",` +
						`"scaling":{"type":"QUEUE_DELAY","queueDelay":4},"workers":{"min":0,"max":3},` +
						`"gpu":{"pools":["ADA_24"]},"registry":"` + keptRegistry + `","dataCenterIds":[],"networkVolumes":[]}`))
				case r.Method == http.MethodGet && r.URL.Path == "/v2/serverless/ep_abc/workers":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"endpointVersion":4,"summary":{"total":0,"running":0,"idle":0,"initializing":0,"throttled":0,"unhealthy":0},"workers":[]}`))
				case r.Method == http.MethodPost && r.URL.Path == "/graphql":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"data":{"myself":{"endpoint":{"userId":"user-1"}}}}`))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer srv.Close()

			r := &serverlessEndpointResource{
				client: shimTestClient(t, srv.URL),
				legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", GraphQLURL: srv.URL + "/graphql"}),
			}
			pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
			if diags.HasError() {
				t.Fatal(diags)
			}
			common := map[string]attr.Value{
				"gpu_pools":     pools,
				"scaling_type":  types.StringValue("QUEUE_DELAY"),
				"scaling_value": types.Float64Value(4),
				"template_id":   types.StringValue("tpl_old"),
			}
			with := func(set map[string]attr.Value) tftypes.Value {
				for name, value := range common {
					set[name] = value
				}
				return shimObject(ctx, t, s, raw, set)
			}
			state := with(map[string]attr.Value{
				"id": types.StringValue("ep_abc"), "name": types.StringValue("worker"), "registry": types.StringValue(keptRegistry),
			})
			plannedRegistry := tc.configured
			if plannedRegistry.IsNull() {
				plannedRegistry = types.StringValue(keptRegistry)
			}
			plan := with(map[string]attr.Value{
				"id": types.StringValue("ep_abc"), "name": types.StringValue("renamed"), "registry": plannedRegistry,
			})
			config := with(map[string]attr.Value{"name": types.StringValue("renamed"), "registry": tc.configured})

			resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
			r.Update(ctx, resource.UpdateRequest{
				Plan:   tfsdk.Plan{Schema: s, Raw: plan},
				State:  tfsdk.State{Schema: s, Raw: state},
				Config: tfsdk.Config{Schema: s, Raw: config},
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if sent == nil {
				t.Fatal("the v2 update was never sent")
			}
			if got := sent["registry"]; got != tc.want {
				t.Errorf("registry sent = %v, want %v", got, tc.want)
			}
		})
	}
}

// planRegistry runs the attribute's modifier chain the way the framework
// does and returns the planned value.
func planRegistry(ctx context.Context, t *testing.T, attribute resourceschema.StringAttribute, req planmodifier.StringRequest) types.String {
	t.Helper()
	for _, modifier := range attribute.PlanModifiers {
		resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
		modifier.PlanModifyString(ctx, req, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		if resp.RequiresReplace {
			t.Error("requires replace, want an in-place update")
		}
		req.PlanValue = resp.PlanValue
	}
	return req.PlanValue
}
