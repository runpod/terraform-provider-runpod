package datasource_endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

func TestEndpointDataSourceRead_Success(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/serverless/ep-1" {
			resp := map[string]interface{}{
				"id": "ep-1",
				"name": "test-endpoint",
				"image": "runpod/pytorch:latest",
				"gpu": map[string]interface{}{
					"pools": []string{"ADA_24"},
					"count": 1,
				},
				"workers": map[string]interface{}{
					"min": 0,
					"max": 5,
				},
				"scaling": map[string]interface{}{
					"type": "QUEUE_DELAY",
					"value": 4.0,
					"idleTimeout": 300,
				},
				"requestUrls": map[string]interface{}{
					"run": "https://api.runpod.ai/v2/ep-1/run",
					"runSync": "https://api.runpod.ai/v2/ep-1/runsync",
				},
				"createdAt": "2026-03-13T20:00:00Z",
				"dataCenterIds": []string{"US-KS-2"},
			}
			respBytes, _ := json.Marshal(resp)
			w.WriteHeader(200)
			w.Write(respBytes)
		}
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	sch := EndpointDataSourceSchema(ctx)
	cfgState := tfsdk.State{Schema: sch}
	diags := cfgState.Set(ctx, &EndpointDataSourceModel{
		Id: types.StringValue("ep-1"),
	})
	if diags.HasError() {
		t.Fatalf("failed to set config: %v", diags)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointDataSource{}).Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: cfgState.Raw}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics, got: %v", resp.Diagnostics)
	}

	var result EndpointDataSourceModel
	diags = resp.State.Get(ctx, &result)
	if diags.HasError() {
		t.Fatalf("failed to get result: %v", diags)
	}

	if result.Name.ValueString() != "test-endpoint" {
		t.Errorf("expected name 'test-endpoint', got %q", result.Name.ValueString())
	}

	if result.RunUrl.ValueString() != "https://api.runpod.ai/v2/ep-1/run" {
		t.Errorf("expected run_url, got %q", result.RunUrl.ValueString())
	}
}

func TestEndpointDataSourceRead_NotFound(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"detail":"not found"}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	sch := EndpointDataSourceSchema(ctx)
	cfgState := tfsdk.State{Schema: sch}
	diags := cfgState.Set(ctx, &EndpointDataSourceModel{
		Id: types.StringValue("nonexistent"),
	})
	if diags.HasError() {
		t.Fatalf("failed to set config: %v", diags)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointDataSource{}).Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: cfgState.Raw}}, resp)

	// On 404, the data source should remove the resource, not return an error
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for 404 (resource removal), got: %v", resp.Diagnostics)
	}

	var result EndpointDataSourceModel
	diags = resp.State.Get(ctx, &result)
	if !diags.HasError() {
		t.Fatal("expected error for removed resource, got none")
	}
}
