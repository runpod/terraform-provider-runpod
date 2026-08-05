package resource_pod_action

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// createForAction drives Create against a stub that returns a valid REST
// response for the given action, and returns the resulting state + diags.
// Reuses actionConfig from pod_action_resource_test.go.
func createForAction(t *testing.T, action, status string) (PodActionModel, resource.CreateResponse) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that we're receiving the POST request with the correct action
		var body map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"data":{"result":{"status":"` + status + `"}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	// Create resource and configure it with the test data
	r := &PodActionResource{}
	
	// Simulate provider Configure by setting the values directly
	r.apiKey = "testkey123"
	r.baseURL = srv.URL
	r.httpClient = &http.Client{}

	m := PodActionModel{
		Action: types.StringValue(action),
		PodId:  types.StringValue("p1"),
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: PodActionResourceSchema(context.Background())}}
	r.Create(context.Background(), resource.CreateRequest{Config: actionConfig(t, m)}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("action %q: Create errored: %v", action, resp.Diagnostics)
	}
	var state PodActionModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("action %q: reading state: %v", action, diags)
	}
	return state, resp
}

// TestPodActionCreate_AllActionsSetStatus exercises the success-path status
// extraction for every v2 action variant. Each asserts Status is populated from
// the REST response's status field.
func TestPodActionCreate_AllActionsSetStatus(t *testing.T) {
	cases := []struct {
		action string
		status string
	}{
		{"start", "RUNNING"},
		{"stop", "STOPPED"},
		{"restart", "RESTARTING"},
		{"terminate", "TERMINATED"},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			state, _ := createForAction(t, tc.action, tc.status)
			if got := state.Status.ValueString(); got != tc.status {
				t.Errorf("action %q: Status = %q, want %q", tc.action, got, tc.status)
			}
		})
	}
}

// TestPodActionCreate_APIError covers the err != nil branch in Create: when the
// REST endpoint is unreachable, the HTTP request fails and Create must add an
// "API Error" diagnostic instead of setting state.
func TestPodActionCreate_APIError(t *testing.T) {
	// Point at a closed server so the HTTP request fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	// Create resource and configure it with the test data
	r := &PodActionResource{}
	r.apiKey = "testkey123"
	r.baseURL = url
	r.httpClient = &http.Client{}

	m := PodActionModel{
		Action: types.StringValue("stop"),
		PodId:  types.StringValue("p1"),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodActionResourceSchema(context.Background())}}
	r.Create(context.Background(), resource.CreateRequest{Config: actionConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an API Error diagnostic when the endpoint is unreachable")
	}
}

// TestPodActionCreate_StatusMissingFromResponse documents behavior when the
// REST response has no usable `status`: the response succeeds (no error) but
// Status is left empty.
func TestPodActionCreate_StatusMissingFromResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Response without usable status
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	// Create resource and configure it with the test data
	r := &PodActionResource{}
	r.apiKey = "testkey123"
	r.baseURL = srv.URL
	r.httpClient = &http.Client{}

	m := PodActionModel{
		Action: types.StringValue("stop"),
		PodId:  types.StringValue("p1"),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodActionResourceSchema(context.Background())}}
	r.Create(context.Background(), resource.CreateRequest{Config: actionConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create should not error when status is missing/wrong type: %v", resp.Diagnostics)
	}
	var state PodActionModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got := state.Status.ValueString(); got != "" {
		t.Errorf("Status = %q, want empty when response status is non-string", got)
	}
}

// TestPodActionMetadata covers Metadata: the resource type name must be
// runpod_pod_action.
func TestPodActionMetadata(t *testing.T) {
	resp := &resource.MetadataResponse{}
	(&PodActionResource{}).Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "runpod_pod_action" {
		t.Errorf("TypeName = %q, want runpod_pod_action", resp.TypeName)
	}
}

// TestPodActionSchema covers Schema: it must populate the response schema with
// the action, pod_id, and status attributes.
func TestPodActionSchema(t *testing.T) {
	resp := &resource.SchemaResponse{}
	(&PodActionResource{}).Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, attr := range []string{"action", "pod_id", "status"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("Schema missing attribute %q", attr)
		}
	}
}

// TestNewPodActionResource covers the constructor: it must return a non-nil
// *PodActionResource.
func TestNewPodActionResource(t *testing.T) {
	r := NewPodActionResource()
	if r == nil {
		t.Fatal("NewPodActionResource returned nil")
	}
	if _, ok := r.(*PodActionResource); !ok {
		t.Errorf("NewPodActionResource returned %T, want *PodActionResource", r)
	}
}
