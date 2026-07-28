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

func actionConfig(t *testing.T, m PodActionModel) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	sch := PodActionResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if diags := st.Set(ctx, &m); diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}
	return tfsdk.Config{Schema: sch, Raw: st.Raw}
}

// TestPodActionCreate_SetsStatus tests that a valid REST response makes Create
// succeed and populate Status from the response's status field.
func TestPodActionCreate_SetsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":{"id":"p1","status":"STOPPED"}}}`))
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
		t.Fatalf("expected CE-1652 fix: Create should succeed, got: %v", resp.Diagnostics)
	}

	var state PodActionModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got := state.Status.ValueString(); got != "STOPPED" {
		t.Errorf("Status = %q, want STOPPED", got)
	}
}

// TestPodActionCreate_SendsCorrectRequest drives Create for all v2-supported
// actions. We capture the request path and body and assert the right action +
// podId went on the wire to POST /v2/pods/{id}/action.
func TestPodActionCreate_SendsCorrectRequest(t *testing.T) {
	cases := []struct {
		action string
	}{
		{"start"},
		{"stop"},
		{"restart"},
		{"terminate"},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			var body map[string]interface{}
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				b, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(b, &body)
				_, _ = w.Write([]byte(`{"data":{"result":{"status":"OK"}}}`))
			}))
			defer srv.Close()

			// Create resource and configure it with the test data
			r := &PodActionResource{}
			r.apiKey = "testkey123"
			r.baseURL = srv.URL
			r.httpClient = &http.Client{}

			m := PodActionModel{
				Action: types.StringValue(tc.action),
				PodId:  types.StringValue("pod-xyz"),
			}
			resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodActionResourceSchema(context.Background())}}
			r.Create(context.Background(), resource.CreateRequest{Config: actionConfig(t, m)}, resp)

			if gotPath != "/v2/pods/pod-xyz/action" {
				t.Errorf("action %q: path = %q, want %q", tc.action, gotPath, "/v2/pods/pod-xyz/action")
			}
			if body["action"] != tc.action {
				t.Errorf("action %q: body.action = %v, want %q", tc.action, body["action"], tc.action)
			}
		})
	}
}

// TestPodActionReadUpdateDelete_NoOp covers the empty Read/Update/Delete stubs:
// pod_action is create-only, so they are intentional no-ops and must not error.
func TestPodActionReadUpdateDelete_NoOp(t *testing.T) {
	ctx := context.Background()
	r := &PodActionResource{}

	rr := &resource.ReadResponse{}
	r.Read(ctx, resource.ReadRequest{}, rr)
	if rr.Diagnostics.HasError() {
		t.Errorf("Read no-op should not error: %v", rr.Diagnostics)
	}
	ur := &resource.UpdateResponse{}
	r.Update(ctx, resource.UpdateRequest{}, ur)
	if ur.Diagnostics.HasError() {
		t.Errorf("Update no-op should not error: %v", ur.Diagnostics)
	}
	dr := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{}, dr)
	if dr.Diagnostics.HasError() {
		t.Errorf("Delete no-op should not error: %v", dr.Diagnostics)
	}
}

func TestPodActionCreate_InvalidAction_Errors(t *testing.T) {
	r := &PodActionResource{}
	r.apiKey = "testkey123"
	r.baseURL = "http://localhost:9999"
	r.httpClient = &http.Client{}

	m := PodActionModel{
		Action: types.StringValue("explode"),
		PodId:  types.StringValue("p1"),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodActionResourceSchema(context.Background())}}
	r.Create(context.Background(), resource.CreateRequest{Config: actionConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an 'Invalid Action' error for an unknown action")
	}
}
