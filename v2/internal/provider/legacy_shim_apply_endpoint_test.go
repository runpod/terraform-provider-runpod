package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// shimEndpointJSON is the endpoint every v2 endpoint route answers with in
// these tests.
const shimEndpointJSON = `{"id":"ep_abc","name":"inference","flashboot":"OFF","createdAt":"2026-09-17T09:00:00Z",` +
	`"scaling":{"type":"QUEUE_DELAY","queueDelay":4},"workers":{"min":0,"max":5},` +
	`"gpu":{"pools":["ADA_24"]},"dataCenterIds":[],"networkVolumes":[]}`

// shimWorkersJSON is a REST v2 workers listing with one running worker.
const shimWorkersJSON = `{"endpointVersion":5,` +
	`"summary":{"total":1,"running":1,"idle":0,"initializing":0,"throttled":0,"unhealthy":0},` +
	`"workers":[{"id":"worker-1","status":"RUNNING","gpuCount":1,"isStale":false,` +
	`"uptimeSeconds":12,"startedAt":"2026-09-17T09:00:00Z","version":5}]}`

// shimOwnerJSON answers the GraphQL endpoint-owner query.
const shimOwnerJSON = `{"data":{"myself":{"endpoint":{"userId":"user-1"}}}}`

// endpointShimAnswers sets what the stub answers on the two deprecated
// sources; a zero workersStatus is 200.
type endpointShimAnswers struct {
	workersStatus int
	workers       string
	graphQL       string
}

// endpointShimRequest is one request the stub received; Body is the decoded
// JSON body, nil for a request without one.
type endpointShimRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

func (r endpointShimRequest) String() string { return r.Method + " " + r.Path }

type endpointShimRecorder struct {
	mu       sync.Mutex
	requests []endpointShimRequest
}

func (r *endpointShimRecorder) record(request endpointShimRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
}

// Requests returns a copy of every request received so far.
func (r *endpointShimRecorder) Requests() []endpointShimRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]endpointShimRequest(nil), r.requests...)
}

// endpointShimResource builds a serverless endpoint resource whose v2 and
// GraphQL clients both talk to one recording httptest server. The server
// serves the v2 endpoint routes, the v2 workers listing and POST /graphql,
// and fails the test on any other path, a REST v1 one included.
func endpointShimResource(t *testing.T, answers endpointShimAnswers) (*serverlessEndpointResource, *endpointShimRecorder) {
	t.Helper()
	recorder := &endpointShimRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := endpointShimRequest{Method: r.Method, Path: r.URL.Path}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading %s: %v", request, err)
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &request.Body); err != nil {
				t.Errorf("decoding %s: %v", request, err)
			}
		}
		recorder.record(request)

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/v1/"):
			t.Errorf("request reached a REST v1 path: %s", request)
			w.WriteHeader(http.StatusGone)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/serverless":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(shimEndpointJSON))
		case (r.Method == http.MethodGet || r.Method == http.MethodPatch) && r.URL.Path == "/v2/serverless/ep_abc":
			_, _ = w.Write([]byte(shimEndpointJSON))
		case r.Method == http.MethodGet && r.URL.Path == "/v2/serverless/ep_abc/workers":
			if answers.workersStatus != 0 {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(answers.workersStatus)
				_, _ = w.Write([]byte(`{"title":"Forbidden","status":403,"detail":"no access"}`))
				return
			}
			_, _ = w.Write([]byte(answers.workers))
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			_, _ = w.Write([]byte(answers.graphQL))
		default:
			t.Errorf("unexpected %s", request)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	return &serverlessEndpointResource{
		client: shimTestClient(t, srv.URL),
		// A plain client: the retrying default would back off for seconds on
		// a failure case.
		legacy: newTestLegacyClient(t, legacy.Config{
			APIKey:     "test-key",
			GraphQLURL: srv.URL + "/graphql",
			HTTPClient: &http.Client{Timeout: 5 * time.Second},
		}),
	}, recorder
}

// assertEndpointShimSequence fails the test unless the stub saw exactly the
// want requests, in order, the GraphQL one being the endpoint-owner query
// for ep_abc, and no request body named a priority.
func assertEndpointShimSequence(t *testing.T, recorder *endpointShimRecorder, want ...string) {
	t.Helper()
	requests := recorder.Requests()
	got := make([]string, 0, len(requests))
	for _, request := range requests {
		got = append(got, request.String())
		if request.Path == "/graphql" {
			variables, _ := request.Body["variables"].(map[string]any)
			if request.Body["query"] != legacyEndpointOwnerQuery || variables["id"] != "ep_abc" {
				t.Errorf("GraphQL request = %v, want the endpoint-owner query for ep_abc", request.Body)
			}
		}
		for key := range request.Body {
			if strings.Contains(strings.ToLower(key), "priority") {
				t.Errorf("%s sent %q: no API applies a priority, so none may be sent", request, key)
			}
		}
	}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

// endpointShimObjects returns the GPU endpoint's tftypes values for the
// given extra attributes: the planned/state object with id, and the
// configuration without it.
func endpointShimObjects(ctx context.Context, t *testing.T, r *serverlessEndpointResource, name string, extra map[string]attr.Value) (tftypes.Value, tftypes.Value) {
	t.Helper()
	s, raw := shimResourceSchema(ctx, t, r)
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	values := map[string]attr.Value{
		"name":          types.StringValue(name),
		"image":         types.StringValue("runpod/worker:1.0.0"),
		"gpu_pools":     pools,
		"scaling_type":  types.StringValue("QUEUE_DELAY"),
		"scaling_value": types.Float64Value(4),
		"workers_max":   types.Int64Value(5),
	}
	for key, value := range extra {
		values[key] = value
	}
	config := shimObject(ctx, t, s, raw, values)
	values["id"] = types.StringValue("ep_abc")
	return shimObject(ctx, t, s, raw, values), config
}

func TestServerlessEndpointCreateReadsTheDeprecatedFieldsFromV2AndGraphQL(t *testing.T) {
	ctx := context.Background()

	r, recorder := endpointShimResource(t, endpointShimAnswers{workers: shimWorkersJSON, graphQL: shimOwnerJSON})
	s, raw := shimResourceSchema(ctx, t, r)
	_, configured := endpointShimObjects(ctx, t, r, "inference", map[string]attr.Value{
		"gpu_type_priority": types.StringValue("availability"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	assertEndpointShimSequence(t, recorder,
		"POST /v2/serverless", "GET /v2/serverless/ep_abc/workers", "POST /graphql")

	var saved serverlessEndpointResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Version.ValueInt64() != 5 {
		t.Errorf("version = %v, want the listing's endpointVersion 5", saved.Version)
	}
	var workers []endpointLegacyWorkerModel
	if diags := saved.Workers.ElementsAs(ctx, &workers, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(workers) != 1 || workers[0].Id.ValueString() != "worker-1" || workers[0].UptimeMs.ValueInt64() != 12000 {
		t.Errorf("workers = %+v, want the one worker the v2 listing reported", workers)
	}
	var users []string
	if diags := saved.Users.ElementsAs(ctx, &users, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(users) != 1 || users[0] != "user-1" {
		t.Errorf("users = %v, want the GraphQL owner", users)
	}
	if !saved.TemplateVersion.IsNull() {
		t.Errorf("template_version = %v, want null: no API reports it", saved.TemplateVersion)
	}
	if saved.GpuTypePriority.ValueString() != "availability" {
		t.Errorf("gpu_type_priority = %v, want the configured value kept", saved.GpuTypePriority)
	}
}

func TestServerlessEndpointUpdateKeepsAPriorityWithoutSendingIt(t *testing.T) {
	ctx := context.Background()

	// A configured priority used to fail the apply after the v2 change had
	// landed. Nothing can apply it now, so the update succeeds, the priority
	// stays in state as configured, and no request carries it.
	r, recorder := endpointShimResource(t, endpointShimAnswers{workers: shimWorkersJSON, graphQL: shimOwnerJSON})
	s, _ := shimResourceSchema(ctx, t, r)
	priority := map[string]attr.Value{"gpu_type_priority": types.StringValue("availability")}
	state, _ := endpointShimObjects(ctx, t, r, "inference", priority)
	plan, config := endpointShimObjects(ctx, t, r, "renamed", priority)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	assertEndpointShimSequence(t, recorder,
		"PATCH /v2/serverless/ep_abc", "GET /v2/serverless/ep_abc/workers", "POST /graphql")
	if patch := recorder.Requests()[0].Body; patch["name"] != "renamed" {
		t.Errorf("v2 PATCH body = %v, want the rename", patch)
	}

	var saved serverlessEndpointResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.GpuTypePriority.ValueString() != "availability" {
		t.Errorf("gpu_type_priority = %v, want the configured value kept", saved.GpuTypePriority)
	}
	if saved.Version.ValueInt64() != 5 || saved.Workers.IsNull() {
		t.Errorf("version = %v, workers = %v, want them read back after the update", saved.Version, saved.Workers)
	}
}

func TestServerlessEndpointUpdateIgnoresUnconfiguredComputeType(t *testing.T) {
	ctx := context.Background()

	// compute_type is Optional+Computed, so an endpoint that never set it
	// still carries the derived value in state and therefore in the plan. It
	// is derived from the compute block, so the update sends nothing for it.
	r, recorder := endpointShimResource(t, endpointShimAnswers{workers: shimWorkersJSON, graphQL: shimOwnerJSON})
	s, _ := shimResourceSchema(ctx, t, r)
	computeType := map[string]attr.Value{"compute_type": types.StringValue("GPU")}
	state, _ := endpointShimObjects(ctx, t, r, "inference", computeType)
	plan, _ := endpointShimObjects(ctx, t, r, "renamed", computeType)
	_, config := endpointShimObjects(ctx, t, r, "renamed", nil)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	assertEndpointShimSequence(t, recorder,
		"PATCH /v2/serverless/ep_abc", "GET /v2/serverless/ep_abc/workers", "POST /graphql")
	if patch := recorder.Requests()[0].Body; patch["computeType"] != nil {
		t.Errorf("v2 PATCH body = %v, want no computeType", patch)
	}
}

func TestServerlessEndpointCreateSurvivesFailedDeprecatedReads(t *testing.T) {
	ctx := context.Background()

	// The canonical 1.x migration shape: gpu_pools plus compute_type, no
	// priority. Nothing it configures depends on the deprecated reads, so a
	// workers listing that refuses and a GraphQL API that errors each warn,
	// leave what they feed null, and the created endpoint lands in state.
	r, recorder := endpointShimResource(t, endpointShimAnswers{
		workersStatus: http.StatusForbidden,
		graphQL:       `{"errors":[{"message":"internal error"}],"data":null}`,
	})
	s, raw := shimResourceSchema(ctx, t, r)
	_, configured := endpointShimObjects(ctx, t, r, "inference", map[string]attr.Value{
		"compute_type": types.StringValue("GPU"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed deprecated reads must not fail the create: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 2 {
		t.Fatalf("the workers read and the owner read must each warn once: %v", resp.Diagnostics)
	}
	assertEndpointShimSequence(t, recorder,
		"POST /v2/serverless", "GET /v2/serverless/ep_abc/workers", "POST /graphql")

	var saved serverlessEndpointResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Id.ValueString() != "ep_abc" || saved.ComputeType.ValueString() != "GPU" {
		t.Fatalf("state = %+v, want the created endpoint with compute_type derived from the v2 block", saved)
	}
	if !saved.Version.IsNull() || !saved.Workers.IsNull() || !saved.Users.IsNull() {
		t.Errorf("version = %v, workers = %v, users = %v, want all null after failed reads",
			saved.Version, saved.Workers, saved.Users)
	}
}

func TestServerlessEndpointReadLeavesUsersNullForAnEndpointGraphQLDoesNotKnow(t *testing.T) {
	ctx := context.Background()

	r, recorder := endpointShimResource(t, endpointShimAnswers{
		workers: shimWorkersJSON,
		graphQL: `{"data":{"myself":{"endpoint":null}}}`,
	})
	s, _ := shimResourceSchema(ctx, t, r)
	state, _ := endpointShimObjects(ctx, t, r, "inference", map[string]attr.Value{
		"template_version": types.Int64Value(7),
	})

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: state}}, resp)
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("an endpoint the GraphQL API does not know is not a failure: %v", resp.Diagnostics)
	}
	assertEndpointShimSequence(t, recorder,
		"GET /v2/serverless/ep_abc", "GET /v2/serverless/ep_abc/workers", "POST /graphql")

	var saved serverlessEndpointResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if !saved.Users.IsNull() {
		t.Errorf("users = %v, want null", saved.Users)
	}
	if !saved.TemplateVersion.IsNull() {
		t.Errorf("template_version = %v, want a stored value cleared: no API reports it", saved.TemplateVersion)
	}
	if saved.Version.ValueInt64() != 5 {
		t.Errorf("version = %v, want 5 from the workers listing", saved.Version)
	}
}
