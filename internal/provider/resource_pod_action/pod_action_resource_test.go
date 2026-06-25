package resource_pod_action

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestPodActionCreate_DoubleUnwrap is a characterization test for bug CE-1652.
// client.Query() already returns the *inner* GraphQL `data` object, but
// PodActionResource.Create does result["data"].(map) again — a second unwrap
// that is always nil. So even a perfectly valid GraphQL response makes Create
// fail with "Failed to get data from response".
//
// This asserts the current (buggy) failure. When CE-1652 is fixed (drop the second
// unwrap), Create will succeed and set Status — flip this test to assert that.
func TestPodActionCreate_DoubleUnwrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"podStop":{"id":"p1","status":"STOPPED"}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	m := PodActionModel{
		Action: types.StringValue("stop"),
		PodId:  types.StringValue("p1"),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodActionResourceSchema(context.Background())}}
	(&PodActionResource{}).Create(context.Background(), resource.CreateRequest{Config: actionConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected CE-1652 double-unwrap failure; if Create now succeeds, CE-1652 is FIXED — flip to assert Status == STOPPED")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "Failed to get data from response") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Failed to get data from response', got: %v", resp.Diagnostics)
	}
}

// TestPodActionCreate_SendsCorrectMutation covers the action→mutation routing
// (the switch in Create) for all four actions, including resume/restart/
// terminate. The request is built and sent before CE-1652's double-unwrap errors, so
// we capture the request body and assert the right GraphQL mutation + podId went
// on the wire. This is real, passing coverage of the routing (independent of the
// CE-1652 response-handling bug, which still fails the Create afterward).
func TestPodActionCreate_SendsCorrectMutation(t *testing.T) {
	cases := []struct {
		action   string
		mutation string
	}{
		{"stop", "podStop"},
		{"resume", "podResume"},
		{"restart", "podRestart"},
		{"terminate", "podTerminate"},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			var body map[string]interface{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(b, &body)
				_, _ = w.Write([]byte(`{"data":{}}`))
			}))
			defer srv.Close()
			t.Setenv("RUNPOD_API_KEY", "testkey123")
			t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

			m := PodActionModel{
				Action: types.StringValue(tc.action),
				PodId:  types.StringValue("pod-xyz"),
			}
			resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodActionResourceSchema(context.Background())}}
			(&PodActionResource{}).Create(context.Background(), resource.CreateRequest{Config: actionConfig(t, m)}, resp)

			q, _ := body["query"].(string)
			if !strings.Contains(q, tc.mutation) {
				t.Errorf("action %q: query did not contain mutation %q; got: %q", tc.action, tc.mutation, q)
			}
			vars, _ := body["variables"].(map[string]interface{})
			if vars["podId"] != "pod-xyz" {
				t.Errorf("action %q: variables.podId = %v, want pod-xyz", tc.action, vars["podId"])
			}
		})
	}
}

func TestPodActionCreate_InvalidAction_Errors(t *testing.T) {
	m := PodActionModel{
		Action: types.StringValue("explode"),
		PodId:  types.StringValue("p1"),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodActionResourceSchema(context.Background())}}
	(&PodActionResource{}).Create(context.Background(), resource.CreateRequest{Config: actionConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an 'Invalid Action' error for an unknown action")
	}
}
