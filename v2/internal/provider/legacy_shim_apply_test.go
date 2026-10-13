package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// The REST v1 shim apply paths, driven through the resources' real Create and
// Update rather than their mapping helpers, because what these pin down is the
// order of the calls: which model the legacy patch is built from, and whether
// the state is written before a legacy failure is reported.
//
// One httptest server stands in for both hosts: the v2 SDK asks for /v2/…
// and the REST v1 shim for everything else, so a single handler sees the whole
// sequence.

func shimTestClient(t *testing.T, url string) *runpod.Client {
	t.Helper()
	client, err := runpod.New(runpod.WithAPIKey("test-key"), runpod.WithBaseURL(url))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// shimResourceSchema constructs a resource's schema and the all-null object
// value plan, config and state start from, the way providerConfig builds a
// provider configuration in legacy_client_test.go.
func shimResourceSchema(ctx context.Context, t *testing.T, r resource.Resource) (resourceschema.Schema, tftypes.Value) {
	t.Helper()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}

	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type = %T, want an object", schemaResp.Schema.Type().TerraformType(ctx))
	}
	values := map[string]tftypes.Value{}
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	return schemaResp.Schema, tftypes.NewValue(objectType, values)
}

// shimObject fills the named attributes into a copy of the all-null object.
func shimObject(ctx context.Context, t *testing.T, s resourceschema.Schema, raw tftypes.Value, set map[string]attr.Value) tftypes.Value {
	t.Helper()
	data := tfsdk.State{Schema: s, Raw: raw}
	for name, value := range set {
		if diags := data.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("setting %s: %v", name, diags)
		}
	}
	return data.Raw
}

func TestTemplateCreateSavesStateOnLegacyWriteFailure(t *testing.T) {
	ctx := context.Background()

	// A template created through v2 that the REST v1 route does not know: the
	// PATCH 404s, which is the concrete way a configured readme fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/templates":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"tpl_abc","name":"trainer","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,"env":{},"ports":[],"allowedCudaVersions":[]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/templates/tpl_abc":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &templateResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":   types.StringValue("trainer"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("# trainer"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a configured readme the legacy host refused must be an error: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the template exists in the account: a failed legacy write must still write state")
	}
	var saved templateResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Id.ValueString() != "tpl_abc" {
		t.Fatalf("state id = %q, want tpl_abc", saved.Id.ValueString())
	}
}

func TestTemplateUpdateSkipsLegacyHostWhenUnconfigured(t *testing.T) {
	ctx := context.Background()

	// The shape terraform core sends on an update of a template that never set
	// readme or docker_entrypoint: the plan carries the prior state value for
	// both, the configuration carries neither. Nothing may be PATCHed to the
	// legacy host from that.
	var patchedV1 bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/templates/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"tpl_abc","name":"renamed","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,"env":{},"ports":[],"allowedCudaVersions":[]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/templates/tpl_abc":
			patchedV1 = true
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/templates/tpl_abc":
			_, _ = w.Write([]byte(`{"readme":"# stale","earned":0,"isRunpod":false,"runtimeInMin":0}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &templateResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	state := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":     types.StringValue("tpl_abc"),
		"name":   types.StringValue("trainer"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("# stale"),
	})
	plan := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":     types.StringValue("tpl_abc"),
		"name":   types.StringValue("renamed"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("# stale"),
	})
	config := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":  types.StringValue("renamed"),
		"image": types.StringValue("runpod/pytorch:1.0.2"),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)

	if patchedV1 {
		t.Fatal("an update that configures no shim input must not write to the deprecated host")
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

func TestServerlessEndpointCreateSavesStateOnLegacyWriteFailure(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/serverless":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"ep_abc","name":"inference","flashboot":"OFF","createdAt":"2026-09-17T09:00:00Z",` +
				`"scaling":{"type":"QUEUE_DELAY","queueDelay":4},"workers":{"min":0,"max":3},` +
				`"gpu":{"pools":["ADA_24"]},"dataCenterIds":[],"networkVolumes":[]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/endpoints/ep_abc":
			w.WriteHeader(http.StatusBadGateway)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &serverlessEndpointResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":              types.StringValue("inference"),
		"image":             types.StringValue("runpod/worker:1.0.0"),
		"gpu_pools":         pools,
		"scaling_type":      types.StringValue("QUEUE_DELAY"),
		"scaling_value":     types.Float64Value(4),
		"gpu_type_priority": types.StringValue("availability"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a configured gpu_type_priority the legacy host refused must be an error: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the endpoint exists and bills: a failed legacy write must still write state")
	}
	var saved serverlessEndpointResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Id.ValueString() != "ep_abc" {
		t.Fatalf("state id = %q, want ep_abc", saved.Id.ValueString())
	}
}

func TestServerlessEndpointUpdateIgnoresUnconfiguredComputeType(t *testing.T) {
	ctx := context.Background()

	// compute_type is Optional+Computed, so an endpoint that never set it
	// still carries the derived value in state and therefore in the plan. It
	// is not configured, so no PATCH may go out.
	var patchedV1 bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/serverless/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ep_abc","name":"renamed","flashboot":"OFF","createdAt":"2026-09-17T09:00:00Z",` +
				`"scaling":{"type":"QUEUE_DELAY","queueDelay":4},"workers":{"min":0,"max":5},` +
				`"gpu":{"pools":["ADA_24"]},"dataCenterIds":[],"networkVolumes":[]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/endpoints/ep_abc":
			patchedV1 = true
			w.WriteHeader(http.StatusBadGateway)
		case r.Method == http.MethodGet && r.URL.Path == "/endpoints/ep_abc":
			_, _ = w.Write([]byte(`{"computeType":"GPU","userId":"user-1","version":4,"workers":[]}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &serverlessEndpointResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	state := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":            types.StringValue("ep_abc"),
		"name":          types.StringValue("inference"),
		"gpu_pools":     pools,
		"scaling_type":  types.StringValue("QUEUE_DELAY"),
		"scaling_value": types.Float64Value(4),
		"workers_max":   types.Int64Value(3),
		"compute_type":  types.StringValue("GPU"),
	})
	plan := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":            types.StringValue("ep_abc"),
		"name":          types.StringValue("renamed"),
		"gpu_pools":     pools,
		"scaling_type":  types.StringValue("QUEUE_DELAY"),
		"scaling_value": types.Float64Value(4),
		"workers_max":   types.Int64Value(5),
		"compute_type":  types.StringValue("GPU"),
	})
	config := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":          types.StringValue("renamed"),
		"gpu_pools":     pools,
		"scaling_type":  types.StringValue("QUEUE_DELAY"),
		"scaling_value": types.Float64Value(4),
		"workers_max":   types.Int64Value(5),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)

	if patchedV1 {
		t.Fatal("an update that configures no shim input must not write to the deprecated host")
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

func TestServerlessEndpointCreateNeverPatchesComputeType(t *testing.T) {
	ctx := context.Background()

	// The canonical 1.x migration shape: gpu_pools plus compute_type, no
	// priority. compute_type is derived from the v2 response and policed
	// against the compute block, so a v1 PATCH carrying it could change
	// nothing, and would fail the whole apply the moment the deprecated host
	// answers 502. Nothing may be written, and the read that follows may only
	// warn.
	var patchedV1 bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/serverless":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"ep_abc","name":"inference","flashboot":"OFF","createdAt":"2026-09-17T09:00:00Z",` +
				`"scaling":{"type":"QUEUE_DELAY","queueDelay":4},"workers":{"min":0,"max":3},` +
				`"gpu":{"pools":["ADA_24"]},"dataCenterIds":[],"networkVolumes":[]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/endpoints/ep_abc":
			patchedV1 = true
			w.WriteHeader(http.StatusBadGateway)
		case r.Method == http.MethodGet && r.URL.Path == "/endpoints/ep_abc":
			w.WriteHeader(http.StatusBadGateway)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &serverlessEndpointResource{
		client: shimTestClient(t, srv.URL),
		// The plain test client: a GET is idempotent, so the shared retry
		// transport would spend seconds backing off before reporting the 502
		// this test is about.
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL, HTTPClient: srv.Client()}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":          types.StringValue("inference"),
		"image":         types.StringValue("runpod/worker:1.0.0"),
		"gpu_pools":     pools,
		"scaling_type":  types.StringValue("QUEUE_DELAY"),
		"scaling_value": types.Float64Value(4),
		"compute_type":  types.StringValue("GPU"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)

	if patchedV1 {
		t.Fatal("a configured compute_type must not reach the deprecated host: the v2 compute block already fixes the family")
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("a compute_type-only configuration must survive a dead legacy host: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 {
		t.Fatalf("the failed read must warn once: %v", resp.Diagnostics)
	}
	var saved serverlessEndpointResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Id.ValueString() != "ep_abc" || saved.ComputeType.ValueString() != "GPU" {
		t.Fatalf("state = %+v, want the created endpoint with compute_type derived from the v2 block", saved)
	}
}

func TestTemplateUpdateSavesStateOnLegacyWriteFailure(t *testing.T) {
	ctx := context.Background()

	// The v2 rename has already landed when the v1 PATCH fails. Returning
	// before State.Set would leave state saying name = "trainer" while the
	// platform says "renamed", and the next plan would see no rename to make.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/templates/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"tpl_abc","name":"renamed","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,"env":{},"ports":[],"allowedCudaVersions":[]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/templates/tpl_abc":
			w.WriteHeader(http.StatusBadGateway)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &templateResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	state := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":     types.StringValue("tpl_abc"),
		"name":   types.StringValue("trainer"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("hi"),
	})
	plan := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":     types.StringValue("tpl_abc"),
		"name":   types.StringValue("renamed"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("hi"),
	})
	config := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":   types.StringValue("renamed"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("hi"),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a configured readme the legacy host refused must be an error: %v", resp.Diagnostics)
	}
	var saved templateResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Name.ValueString() != "renamed" {
		t.Fatalf("state name = %q, want the rename the v2 update already applied", saved.Name.ValueString())
	}
}

func TestServerlessEndpointUpdateSavesStateOnLegacyWriteFailure(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/serverless/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ep_abc","name":"renamed","flashboot":"OFF","createdAt":"2026-09-17T09:00:00Z",` +
				`"scaling":{"type":"QUEUE_DELAY","queueDelay":4},"workers":{"min":0,"max":5},` +
				`"gpu":{"pools":["ADA_24"]},"dataCenterIds":[],"networkVolumes":[]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/endpoints/ep_abc":
			w.WriteHeader(http.StatusBadGateway)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &serverlessEndpointResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	state := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":                types.StringValue("ep_abc"),
		"name":              types.StringValue("inference"),
		"gpu_pools":         pools,
		"scaling_type":      types.StringValue("QUEUE_DELAY"),
		"scaling_value":     types.Float64Value(4),
		"workers_max":       types.Int64Value(3),
		"gpu_type_priority": types.StringValue("availability"),
	})
	plan := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":                types.StringValue("ep_abc"),
		"name":              types.StringValue("renamed"),
		"gpu_pools":         pools,
		"scaling_type":      types.StringValue("QUEUE_DELAY"),
		"scaling_value":     types.Float64Value(4),
		"workers_max":       types.Int64Value(5),
		"gpu_type_priority": types.StringValue("availability"),
	})
	config := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":              types.StringValue("renamed"),
		"gpu_pools":         pools,
		"scaling_type":      types.StringValue("QUEUE_DELAY"),
		"scaling_value":     types.Float64Value(4),
		"workers_max":       types.Int64Value(5),
		"gpu_type_priority": types.StringValue("availability"),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a configured gpu_type_priority the legacy host refused must be an error: %v", resp.Diagnostics)
	}
	var saved serverlessEndpointResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Name.ValueString() != "renamed" || saved.WorkersMax.ValueInt64() != 5 {
		t.Fatalf("state = %+v, want what the v2 update already applied", saved)
	}
}

func TestPodUpdateSavesStateWhenDesiredStateFails(t *testing.T) {
	ctx := context.Background()

	// The v2 PATCH has already applied the rename and the new env when the
	// action is refused, so the state has to carry them: returning first would
	// hand the next plan a pod it thinks is unchanged.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/v2/pods/pod_abc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"pod_abc","name":"renamed","image":"runpod/pytorch:1.0.2","status":"PROVISIONING",` +
				`"createdAt":"2026-09-17T09:00:00Z","disk":50,"env":{"MODEL":"mistral"},"ports":[],"dataCenterIds":[],"cost":0.35}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/pods/pod_abc/action":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"detail":"pod is still provisioning"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pods/pod_abc":
			_, _ = w.Write([]byte(`{"machineId":"machine-1","volumeEncrypted":true}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &podResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	state := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":            types.StringValue("pod_abc"),
		"name":          types.StringValue("trainer"),
		"image":         types.StringValue("runpod/pytorch:1.0.2"),
		"gpu_type_id":   types.StringValue("NVIDIA A100 80GB PCIe"),
		"status":        types.StringValue("PROVISIONING"),
		"desired_state": types.StringValue("EXITED"),
	})
	plan := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":            types.StringValue("pod_abc"),
		"name":          types.StringValue("renamed"),
		"image":         types.StringValue("runpod/pytorch:1.0.2"),
		"gpu_type_id":   types.StringValue("NVIDIA A100 80GB PCIe"),
		"desired_state": types.StringValue("EXITED"),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: plan},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a refused state change must be reported: %v", resp.Diagnostics)
	}
	var saved podResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Name.ValueString() != "renamed" {
		t.Fatalf("state name = %q, want the rename the v2 update already applied", saved.Name.ValueString())
	}
	if saved.MachineId.ValueString() != "machine-1" {
		t.Fatalf("state = %+v, want the shim read-backs settled before the state was written", saved)
	}
}

func TestPodCreateSavesStateWhenDesiredStateFails(t *testing.T) {
	ctx := context.Background()

	// desired_state = EXITED stops the pod straight out of PROVISIONING, which
	// is the status the API is most likely to refuse. The pod exists and bills
	// either way, so the failure must not cost the state entry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/pods":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"pod_abc","name":"trainer","image":"runpod/pytorch:1.0.2","status":"PROVISIONING",` +
				`"createdAt":"2026-09-17T09:00:00Z","disk":50,"env":{},"ports":[],"dataCenterIds":[],"cost":0.35}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/pods/pod_abc/action":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"detail":"pod is still provisioning"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pods/pod_abc":
			_, _ = w.Write([]byte(`{"machineId":"machine-1","volumeEncrypted":true}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &podResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":          types.StringValue("trainer"),
		"image":         types.StringValue("runpod/pytorch:1.0.2"),
		"gpu_type_id":   types.StringValue("NVIDIA A100 80GB PCIe"),
		"desired_state": types.StringValue("EXITED"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a refused state change must be reported: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the pod exists and bills: a refused state change must still write state")
	}
	var saved podResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Id.ValueString() != "pod_abc" {
		t.Fatalf("state id = %q, want pod_abc", saved.Id.ValueString())
	}
	// The shim read-backs are settled before the state is written, so the
	// saved entry is complete rather than half-filled.
	if saved.MachineId.ValueString() != "machine-1" || !saved.VolumeEncrypted.ValueBool() {
		t.Fatalf("shim read-backs: %+v", saved)
	}
}

func TestTemplateCreateKeepsReadmeOnLegacyWriteFailure(t *testing.T) {
	ctx := context.Background()

	// The same 404 as above, read from the other end: what the failed write
	// leaves in state. A null readme there would put state at odds with the
	// configuration on every later plan, and every later apply would re-run
	// this same failing write. The 404 read path is silent, so nothing would
	// ever say why.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/templates":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"tpl_abc","name":"trainer","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,"env":{},"ports":[],"allowedCudaVersions":[]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/templates/tpl_abc":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &templateResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":   types.StringValue("trainer"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("# trainer"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)

	var saved templateResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Readme.ValueString() != "# trainer" {
		t.Fatalf("state readme = %v, want the configured value so the next plan converges", saved.Readme)
	}
	if !saved.Earned.IsNull() || !saved.IsRunpod.IsNull() || !saved.RuntimeInMin.IsNull() {
		t.Fatalf("the read-backs the failed call never answered must be null: %+v", saved)
	}
	if !resp.Diagnostics.HasError() {
		t.Fatalf("a configured readme the legacy host refused is still an error: %v", resp.Diagnostics)
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "does not know this template") {
		t.Fatalf("the error must name the cause rather than relay a bare 404: %q", detail)
	}
}

func TestServerlessEndpointUpdateReadsWorkersBackAfterAPriorityPatch(t *testing.T) {
	ctx := context.Background()

	// A configured gpu_type_priority PATCHes the deprecated host. The v1 spec
	// gives that route no includeWorkers parameter, so here its response
	// carries no workers, the way GET does without the parameter. The
	// read-backs have to come from the GET that asks for them, or workers
	// would read empty after every apply and filled after the next refresh.
	var patched, read bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/serverless/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ep_abc","name":"inference","flashboot":"OFF","createdAt":"2026-09-17T09:00:00Z",` +
				`"scaling":{"type":"QUEUE_DELAY","queueDelay":4},"workers":{"min":0,"max":5},` +
				`"gpu":{"pools":["ADA_24"]},"dataCenterIds":[],"networkVolumes":[]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/endpoints/ep_abc":
			patched = true
			_, _ = w.Write([]byte(`{"userId":"user-1","version":4}`))
		case r.Method == http.MethodGet && r.URL.Path == "/endpoints/ep_abc":
			if !patched {
				t.Error("the read-back GET went out before the PATCH")
			}
			read = true
			body := `{"userId":"user-1","version":5`
			if r.URL.Query().Get("includeWorkers") == "true" {
				body += `,"workers":[{"id":"worker-1","podId":"pod-1","status":"RUNNING"}]`
			}
			_, _ = w.Write([]byte(body + `}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &serverlessEndpointResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	values := map[string]attr.Value{
		"id":                types.StringValue("ep_abc"),
		"name":              types.StringValue("inference"),
		"gpu_pools":         pools,
		"scaling_type":      types.StringValue("QUEUE_DELAY"),
		"scaling_value":     types.Float64Value(4),
		"workers_max":       types.Int64Value(5),
		"gpu_type_priority": types.StringValue("availability"),
	}
	state := shimObject(ctx, t, s, raw, values)
	plan := shimObject(ctx, t, s, raw, values)
	delete(values, "id")
	config := shimObject(ctx, t, s, raw, values)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !patched || !read {
		t.Fatalf("patched = %v, read = %v, want the PATCH and then the GET", patched, read)
	}

	var saved serverlessEndpointResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Workers.IsNull() || len(saved.Workers.Elements()) != 1 {
		t.Errorf("workers = %v, want the one worker the GET reported", saved.Workers)
	}
	if saved.Version.ValueInt64() != 5 {
		t.Errorf("version = %v, want the GET's 5, not the PATCH response's 4", saved.Version)
	}
	if saved.GpuTypePriority.ValueString() != "availability" {
		t.Errorf("gpu_type_priority = %v, want the configured value kept", saved.GpuTypePriority)
	}
}
