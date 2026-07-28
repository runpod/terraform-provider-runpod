package datasource_gpu_types

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// TestGpuTypesRead_PopulatesState tests the v2 REST endpoint migration
// The endpoint now uses GET /v2/gpu instead of GraphQL
func TestGpuTypesRead_PopulatesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// Verify request method and path
	if r.Method != "GET" {
		t.Errorf("expected GET request, got %q", r.Method)
	}
	if r.URL.Path != "/v2/catalog/gpus" {
		t.Errorf("expected path /v2/catalog/gpus, got %q", r.URL.Path)
	}
		// Verify authorization header
		if auth := r.Header.Get("Authorization"); auth != "Bearer testkey123" {
			t.Errorf("expected Bearer token, got %q", auth)
		}
		// Return v2 REST format with "data" envelope
		_, _ = w.Write([]byte(`{"data":{"gpus":[{"id":"g1","displayName":"A100","manufacturer":"NVIDIA","cuda_cores":6912,"memory_in_gb":80,"community_price":1.0,"secure_price":2.0,"secure_cloud":true}]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: GpuTypesDataSourceSchema(ctx)}}
	(&GpuTypesDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to succeed, got diags=%v", resp.Diagnostics)
	}

	var state GpuTypesDataSourceModel
	diags := resp.State.Get(ctx, &state)
	if diags.HasError() {
		t.Fatalf("expected to read state back, got diags=%v", diags)
	}
	if len(state.GpuTypes) != 1 {
		t.Fatalf("expected 1 GPU type, got %d", len(state.GpuTypes))
	}
	if state.GpuTypes[0].Id != types.StringValue("g1") {
		t.Errorf("GPU type ID: want %q, got %v", "g1", state.GpuTypes[0].Id)
	}
	if state.GpuTypes[0].DisplayName != types.StringValue("A100") {
		t.Errorf("GPU type name: want %q, got %v", "A100", state.GpuTypes[0].DisplayName)
	}
}

// TestGpuTypesRead_MultipleGpus tests parsing multiple GPU types
func TestGpuTypesRead_MultipleGpus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"gpus":[
			{"id":"a100","displayName":"A100","manufacturer":"NVIDIA","cuda_cores":6912,"memory_in_gb":80,"community_price":1.5,"secure_price":2.5,"secure_cloud":true},
			{"id":"v100","displayName":"V100","manufacturer":"NVIDIA","cuda_cores":5120,"memory_in_gb":32,"community_price":1.0,"secure_price":2.0,"secure_cloud":false}
		]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: GpuTypesDataSourceSchema(ctx)}}
	(&GpuTypesDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to succeed, got diags=%v", resp.Diagnostics)
	}

	var state GpuTypesDataSourceModel
	diags := resp.State.Get(ctx, &state)
	if diags.HasError() {
		t.Fatalf("expected to read state back, got diags=%v", diags)
	}
	if len(state.GpuTypes) != 2 {
		t.Fatalf("expected 2 GPU types, got %d", len(state.GpuTypes))
	}
	if state.GpuTypes[0].Id != types.StringValue("a100") {
		t.Errorf("First GPU ID: want %q, got %v", "a100", state.GpuTypes[0].Id)
	}
	if state.GpuTypes[1].Id != types.StringValue("v100") {
		t.Errorf("Second GPU ID: want %q, got %v", "v100", state.GpuTypes[1].Id)
	}
}

// TestGpuTypesRead_ApiError tests API error handling
func TestGpuTypesRead_ApiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal Server Error"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: GpuTypesDataSourceSchema(ctx)}}
	(&GpuTypesDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to fail with API error")
	}
}

// TestGpuTypesRead_MissingGpusField tests handling of missing gpus field
func TestGpuTypesRead_MissingGpusField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"meta":{"version":"2.0"}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: GpuTypesDataSourceSchema(ctx)}}
	(&GpuTypesDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to fail when gpus field is missing")
	}
}
