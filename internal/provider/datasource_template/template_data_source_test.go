package datasource_template

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestTemplateDataSourceRead_PopulatesState asserts the CORRECT behavior of the
// template data source Read: given a valid config (id) and a valid v2 REST
// response, Read decodes the config, issues the REST request, handles the
// v2 response envelope, and populates state (name / image / etc.) with no
// diagnostics error.
//
func TestTemplateDataSourceRead_PopulatesState(t *testing.T) {

	// Valid v2 REST response: The data source uses GET /v2/templates/{id} which
	// returns {data: {template: {...}}} envelope, and the data source extracts
	// the template object directly from data.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "GET" {
			t.Errorf("method: got %q, want GET", r.Method)
		}
		if r.URL.Path != "/v2/templates/tmpl-123" {
			t.Errorf("path: got %q, want /v2/templates/tmpl-123", r.URL.Path)
		}

		// Check Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer testkey123" {
			t.Errorf("Authorization: got %q, want Bearer testkey123", auth)
		}

		// Return v2 REST response format with data envelope
		_, _ = w.Write([]byte(`{"data":{"template":{
			"id":"tmpl-123",
			"name":"my-template",
			"image":"runpod/base:latest",
			"category":"NVIDIA",
			"disk":20,
			"containerRegistryAuthId":"auth-1",
			"entrypoint":["/bin/bash"],
			"cmd":["start.sh"],
			"env":{"FOO":"bar"},
			"public":true,
			"serverless":false,
			"ports":["8888/http"],
			"readme":"hello",
			"volumeInGb":50,
			"mountPath":"/workspace",
			"earned":1.5,
			"isRunpod":true,
			"runtimeInMin":10
		}}}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	sch := TemplateDataSourceSchema(ctx)

	// Build a well-formed config with id set and the computed fields null.
	objType := sch.Type().TerraformType(ctx).(tftypes.Object)
	vals := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, typ := range objType.AttributeTypes {
		if name == "id" {
			vals[name] = tftypes.NewValue(typ, "tmpl-123")
		} else if name == "env" {
			// env is now a Map type
			if mapType, ok := typ.(tftypes.Map); ok {
				vals[name] = tftypes.NewValue(mapType, map[string]tftypes.Value{})
			} else {
				vals[name] = tftypes.NewValue(typ, nil)
			}
		} else {
			vals[name] = tftypes.NewValue(typ, nil)
		}
	}
	rawCfg := tftypes.NewValue(objType, vals)

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: rawCfg}}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}

	(&TemplateDataSource{}).Read(ctx, req, resp)

	// CORRECT: Read completes with no error.
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to succeed, got diags=%v", resp.Diagnostics)
	}

	// CORRECT: state is populated from the v2 REST response.
	var state TemplateModel
	diags := resp.State.Get(ctx, &state)
	if diags.HasError() {
		t.Fatalf("expected to read state back, got diags=%v", diags)
	}
	if state.Name != types.StringValue("my-template") {
		t.Errorf("name: want %q, got %v", "my-template", state.Name)
	}
	if state.ImageName != types.StringValue("runpod/base:latest") {
		t.Errorf("imageName: want %q, got %v", "runpod/base:latest", state.ImageName)
	}
}

// TestTemplateDataSourceRead_404Error handles the case where template is not found
func TestTemplateDataSourceRead_404Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Template not found"}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	sch := TemplateDataSourceSchema(ctx)

	objType := sch.Type().TerraformType(ctx).(tftypes.Object)
	vals := make(map[string]tftypes.Value)
	for name, typ := range objType.AttributeTypes {
		if name == "id" {
			vals[name] = tftypes.NewValue(typ, "nonexistent")
		} else {
			vals[name] = tftypes.NewValue(typ, nil)
		}
	}
	rawCfg := tftypes.NewValue(objType, vals)

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: rawCfg}}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}

	(&TemplateDataSource{}).Read(ctx, req, resp)

	// Should have error for 404
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to fail with 404, got no error")
	}
}

// TestTemplateDataSourceRead_MissingFields handles missing required fields
func TestTemplateDataSourceRead_MissingFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Response missing required 'name' field
		_, _ = w.Write([]byte(`{"data":{"template":{
			"id":"tmpl-123",
			"image":"runpod/base:latest",
			"category":"NVIDIA",
			"disk":20,
			"containerRegistryAuthId":"auth-1",
			"entrypoint":[],
			"cmd":[],
			"env":{},
			"public":true,
			"serverless":false,
			"ports":[],
			"readme":"hello",
			"volumeInGb":50,
			"mountPath":"/workspace",
			"earned":1.5,
			"isRunpod":true,
			"runtimeInMin":10
		}}}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	sch := TemplateDataSourceSchema(ctx)

	objType := sch.Type().TerraformType(ctx).(tftypes.Object)
	vals := make(map[string]tftypes.Value)
	for name, typ := range objType.AttributeTypes {
		if name == "id" {
			vals[name] = tftypes.NewValue(typ, "tmpl-123")
		} else {
			vals[name] = tftypes.NewValue(typ, nil)
		}
	}
	rawCfg := tftypes.NewValue(objType, vals)

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: rawCfg}}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}

	(&TemplateDataSource{}).Read(ctx, req, resp)

	// Should have error for missing 'name' field
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to fail with missing name, got no error")
	}
}
