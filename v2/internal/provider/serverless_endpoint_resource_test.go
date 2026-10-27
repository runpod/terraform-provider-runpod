package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

func endpointGlobalVolumes(ctx context.Context, t *testing.T, ids ...string) types.List {
	t.Helper()
	// A nil slice would become a null list; [] in a configuration is an empty
	// one.
	value, diags := types.ListValueFrom(ctx, types.StringType, append([]string{}, ids...))
	if diags.HasError() {
		t.Fatal(diags)
	}
	return value
}

func TestServerlessEndpointCreateSendsTheGlobalVolume(t *testing.T) {
	// A global volume goes out under mounts.global, as a volume id only: the
	// API mounts it at /runpod-volume.
	ctx := context.Background()
	r, recorder := endpointShimResource(t, endpointShimAnswers{workers: shimWorkersJSON, graphQL: shimOwnerJSON})
	s, raw := shimResourceSchema(ctx, t, r)
	_, configured := endpointShimObjects(ctx, t, r, "inference", map[string]attr.Value{
		"global_volumes": endpointGlobalVolumes(ctx, t, "gsb_abc"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: configured}, Config: tfsdk.Config{Schema: s, Raw: configured}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	assertEndpointShimSequence(t, recorder,
		"POST /v2/serverless", "GET /v2/serverless/ep_abc/workers", "POST /graphql")
	create := recorder.Requests()[0].Body
	if encoded, _ := json.Marshal(create["mounts"]); string(encoded) != `{"global":[{"volumeId":"gsb_abc"}]}` {
		t.Errorf("mounts = %s, want the one global volume", encoded)
	}
	if _, sent := create["networkVolumes"]; sent {
		t.Errorf("create body = %v, want no networkVolumes", create)
	}
}

func TestServerlessEndpointUpdateDetachesTheGlobalVolume(t *testing.T) {
	// [] detaches: the PATCH carries mounts.global as an empty list. An
	// unconfigured list sends no mounts at all, which keeps them.
	ctx := context.Background()
	for _, testCase := range []struct {
		name    string
		planned attr.Value
		want    string
	}{
		{"detach", endpointGlobalVolumes(ctx, t), `{"global":[]}`},
		{"unconfigured", types.ListUnknown(types.StringType), `null`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			r, recorder := endpointShimResource(t, endpointShimAnswers{workers: shimWorkersJSON, graphQL: shimOwnerJSON})
			s, _ := shimResourceSchema(ctx, t, r)
			state, _ := endpointShimObjects(ctx, t, r, "inference", map[string]attr.Value{
				"global_volumes": endpointGlobalVolumes(ctx, t, "gsb_abc"),
			})
			plan, config := endpointShimObjects(ctx, t, r, "inference", map[string]attr.Value{"global_volumes": testCase.planned})

			resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
			r.Update(ctx, resource.UpdateRequest{
				Plan:   tfsdk.Plan{Schema: s, Raw: plan},
				State:  tfsdk.State{Schema: s, Raw: state},
				Config: tfsdk.Config{Schema: s, Raw: config},
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			patch := recorder.Requests()[0]
			if patch.String() != "PATCH /v2/serverless/ep_abc" {
				t.Fatalf("first request = %s, want the PATCH", patch)
			}
			if encoded, _ := json.Marshal(patch.Body["mounts"]); string(encoded) != testCase.want {
				t.Errorf("mounts = %s, want %s", encoded, testCase.want)
			}
		})
	}
}

func TestFlattenEndpointReadsTheGlobalVolume(t *testing.T) {
	ctx := context.Background()
	var model serverlessEndpointResourceModel
	endpoint := &sdktypes.Endpoint{}
	if err := json.Unmarshal([]byte(shimEndpointJSON), endpoint); err != nil {
		t.Fatal(err)
	}
	endpoint.Mounts = sdktypes.EndpointMounts{Global: []sdktypes.EndpointMount{{VolumeId: "gsb_abc"}}}
	if diags := flattenEndpoint(ctx, endpoint, &model); diags.HasError() {
		t.Fatal(diags)
	}
	var ids []string
	if diags := model.GlobalVolumes.ElementsAs(ctx, &ids, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(ids) != 1 || ids[0] != "gsb_abc" {
		t.Errorf("global_volumes = %v, want [gsb_abc]", ids)
	}

	endpoint.Mounts.Global = nil
	if diags := flattenEndpoint(ctx, endpoint, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.GlobalVolumes.IsNull() || len(model.GlobalVolumes.Elements()) != 0 {
		t.Errorf("an endpoint without a global volume reads an empty list, got %v", model.GlobalVolumes)
	}
}

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
		case r.Method == http.MethodGet && r.URL.Path == "/v2/serverless/ep_abc/workers":
			// The deprecated workers and version read-backs.
			_, _ = w.Write([]byte(`{"endpointVersion":1,"summary":{"total":0,"running":0,"idle":0,"initializing":0,"throttled":0,"unhealthy":0},"workers":[]}`))
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
