package provider

import (
	"context"
	"encoding/json"
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

// podShimRequests records every request a podShimServer received. The handler
// goroutine and the test goroutine both touch it, so every access goes
// through the mutex.
type podShimRequests struct {
	mu         sync.Mutex
	paths      []string
	podQueries int
}

func (p *podShimRequests) record(path string, podQuery bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paths = append(p.paths, path)
	if podQuery {
		p.podQueries++
	}
}

// assertPodQueries fails the test unless the GraphQL pod query was sent want
// times and no request reached a REST v1 path.
func (p *podShimRequests) assertPodQueries(t *testing.T, want int) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.podQueries != want {
		t.Errorf("GraphQL pod queries = %d, want %d", p.podQueries, want)
	}
	for _, path := range p.paths {
		if strings.Contains(path, "/v1/") {
			t.Errorf("request reached a REST v1 path: %s", path)
		}
	}
}

// podShimServer stands in for both hosts a pod apply talks to. v2 serves the
// REST v2 routes and reports whether it handled the request; POST /graphql
// must carry legacyPodFieldsQuery for pod_abc and is answered with graphQL,
// a whole GraphQL response body. Any other request fails the test. The
// returned legacy client posts to that GraphQL path with a plain HTTP client,
// so a failure case does not sit through the retrying default's back-off.
func podShimServer(t *testing.T, graphQL string, v2 func(w http.ResponseWriter, r *http.Request) bool) (*httptest.Server, *legacy.Client, *podShimRequests) {
	t.Helper()
	requests := &podShimRequests{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/graphql" {
			var request struct {
				Query     string         `json:"query"`
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decoding the GraphQL request: %v", err)
			}
			if request.Query != legacyPodFieldsQuery {
				t.Errorf("GraphQL document = %q, want legacyPodFieldsQuery", request.Query)
			}
			if len(request.Variables) != 1 || request.Variables["podId"] != "pod_abc" {
				t.Errorf("GraphQL variables = %v, want only podId pod_abc", request.Variables)
			}
			requests.record(r.URL.Path, true)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(graphQL))
			return
		}
		requests.record(r.URL.Path, false)
		if !v2(w, r) {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	client := newTestLegacyClient(t, legacy.Config{
		APIKey:     "test-key",
		GraphQLURL: srv.URL + "/graphql",
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	})
	return srv, client, requests
}

// podShimFieldsAnswer is the GraphQL answer for a pod on machine-1 with an
// encrypted volume.
const podShimFieldsAnswer = `{"data":{"pod":{"machineId":"machine-1","volumeEncrypted":true}}}`

func TestPodUpdateSavesStateWhenDesiredStateFails(t *testing.T) {
	ctx := context.Background()

	// The v2 PATCH has already applied the rename and the new env when the
	// action is refused, so the state has to carry them: returning first would
	// hand the next plan a pod it thinks is unchanged.
	srv, legacyClient, requests := podShimServer(t, podShimFieldsAnswer, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/v2/pods/pod_abc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"pod_abc","name":"renamed","image":"runpod/pytorch:1.0.2","status":"PROVISIONING",` +
				`"createdAt":"2026-09-17T09:00:00Z","disk":50,"env":{"MODEL":"mistral"},"ports":[],"dataCenterIds":[],"cost":0.35}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/pods/pod_abc/action":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"detail":"pod is still provisioning"}`))
		default:
			return false
		}
		return true
	})

	r := &podResource{client: shimTestClient(t, srv.URL), legacy: legacyClient}
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
	if saved.MachineId.ValueString() != "machine-1" || !saved.VolumeEncrypted.ValueBool() {
		t.Fatalf("state = %+v, want the shim read-backs settled before the state was written", saved)
	}
	requests.assertPodQueries(t, 1)
}

func TestPodCreateSavesStateWhenDesiredStateFails(t *testing.T) {
	ctx := context.Background()

	// desired_state = EXITED stops the pod straight out of PROVISIONING, which
	// is the status the API is most likely to refuse. The pod exists and bills
	// either way, so the failure must not cost the state entry.
	srv, legacyClient, requests := podShimServer(t, podShimFieldsAnswer, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/pods":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"pod_abc","name":"trainer","image":"runpod/pytorch:1.0.2","status":"PROVISIONING",` +
				`"createdAt":"2026-09-17T09:00:00Z","disk":50,"env":{},"ports":[],"dataCenterIds":[],"cost":0.35}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/pods/pod_abc/action":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"detail":"pod is still provisioning"}`))
		default:
			return false
		}
		return true
	})

	r := &podResource{client: shimTestClient(t, srv.URL), legacy: legacyClient}
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
	requests.assertPodQueries(t, 1)
}

func TestPodCreateKeepsTheV2PodWhenTheGraphQLReadFails(t *testing.T) {
	ctx := context.Background()

	// Nothing the configuration asks for depends on the GraphQL API, so a
	// failed read must not fail the apply: it warns and the two read-backs
	// are null in the state the create writes.
	srv, legacyClient, requests := podShimServer(t, `{"errors":[{"message":"pod lookup unavailable"}]}`,
		func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method != http.MethodPost || r.URL.Path != "/v2/pods" {
				return false
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"pod_abc","name":"trainer","image":"runpod/pytorch:1.0.2","status":"RUNNING",` +
				`"createdAt":"2026-09-17T09:00:00Z","disk":50,"env":{},"ports":[],"dataCenterIds":[],"cost":0.35}`))
			return true
		})

	r := &podResource{client: shimTestClient(t, srv.URL), legacy: legacyClient}
	s, raw := shimResourceSchema(ctx, t, r)
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":        types.StringValue("trainer"),
		"image":       types.StringValue("runpod/pytorch:1.0.2"),
		"gpu_type_id": types.StringValue("NVIDIA A100 80GB PCIe"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a failed GraphQL read must not fail the create: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 ||
		!strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), "pod lookup unavailable") {
		t.Fatalf("want one warning carrying the GraphQL error, got %v", resp.Diagnostics)
	}
	var saved podResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Id.ValueString() != "pod_abc" {
		t.Fatalf("state id = %q, want pod_abc", saved.Id.ValueString())
	}
	if !saved.MachineId.IsNull() || !saved.VolumeEncrypted.IsNull() {
		t.Fatalf("shim read-backs = %v / %v, want null after a failed read", saved.MachineId, saved.VolumeEncrypted)
	}
	requests.assertPodQueries(t, 1)
}
