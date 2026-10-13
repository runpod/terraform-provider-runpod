package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// endpointJSON is a GPU endpoint as REST v2 serializes it when nothing is
// excluded: the gpu block carries no excludedTypes key at all.
const endpointJSON = `{"id":"ep_abc","name":"inference","flashboot":"OFF","createdAt":"2026-09-17T09:00:00Z",` +
	`"scaling":{"type":"QUEUE_DELAY","queueDelay":4},"workers":{"min":0,"max":3},` +
	`"gpu":{"pools":["ADA_24"],"allowedCudaVersions":[]},"env":{"MODEL":"mistral","HF_HOME":"/workspace/hf"},` +
	`"dataCenterIds":[],"networkVolumes":[]}`

func endpointServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/serverless":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(endpointJSON))
		case r.Method == http.MethodGet && r.URL.Path == "/v2/serverless/ep_abc":
			_, _ = w.Write([]byte(endpointJSON))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestServerlessEndpointCreateKeepsAnEmptyExcludedTypes(t *testing.T) {
	ctx := context.Background()

	// excluded_types = [] states "exclude nothing". The API answers that by
	// omitting the field, which used to read back as null and fail the apply
	// with an inconsistent result.
	r := &serverlessEndpointResource{client: shimTestClient(t, endpointServer(t).URL)}
	s, raw := shimResourceSchema(ctx, t, r)
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":           types.StringValue("inference"),
		"image":          types.StringValue("runpod/worker:1.0.0"),
		"endpoint_type":  types.StringValue("QUEUE"),
		"gpu_pools":      pools,
		"excluded_types": types.ListValueMust(types.StringType, []attr.Value{}),
		"scaling_type":   types.StringValue("QUEUE_DELAY"),
		"scaling_value":  types.Float64Value(4),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}

	var saved serverlessEndpointResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.ExcludedTypes.IsNull() || len(saved.ExcludedTypes.Elements()) != 0 {
		t.Errorf("excluded_types = %v, want the configured []", saved.ExcludedTypes)
	}
}

func TestServerlessEndpointImportFillsTheEnv(t *testing.T) {
	ctx := context.Background()

	r := &serverlessEndpointResource{client: shimTestClient(t, endpointServer(t).URL)}
	s, raw := shimResourceSchema(ctx, t, r)

	importResp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "ep_abc"}, importResp)
	readResp := &resource.ReadResponse{State: importResp.State}
	r.Read(ctx, resource.ReadRequest{State: importResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatal(readResp.Diagnostics)
	}

	var saved serverlessEndpointResourceModel
	if diags := readResp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	var env map[string]string
	if diags := saved.Env.ElementsAs(ctx, &env, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(env) != 2 || env["MODEL"] != "mistral" || env["HF_HOME"] != "/workspace/hf" {
		t.Errorf("imported env = %v, want the endpoint's env", saved.Env)
	}
}
