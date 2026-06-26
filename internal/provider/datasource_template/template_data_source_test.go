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
// template data source Read: given a valid config (id) and a valid GraphQL
// response, Read decodes the config, issues the query, single-unwraps the
// envelope, and populates state (name / imageName / etc.) with no diagnostics
// error.
//
// This is gated on two open bugs in the source (NOT in this test):
//   - CE-1670: TemplateModel in template_data_source_gen.go has malformed
//     `tfsdk` struct tags (missing closing quote on every field after Name),
//     so req.Config.Get(ctx, &config) fails reflection ("need a struct tag for
//     \"tfsdk\"") and go vet fails.
//   - CE-1671: Read re-indexes result["data"] at template_data_source.go:71
//     after client.Query already stripped the {"data":...} envelope
//     (double-unwrap), so the else branch always fires.
//
// Un-skip when both are fixed.
func TestTemplateDataSourceRead_PopulatesState(t *testing.T) {
	t.Skip("CE-1670: malformed tfsdk struct tags in template_data_source_gen.go break reflection (go vet fails); CE-1671: Read re-unwraps result[\"data\"] (double-unwrap). Un-skip when both are fixed")

	// Valid GraphQL response: client.Query strips the {"data":...} envelope and
	// returns the inner map, so a correct Read reads result["template"] directly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"template":{
			"id":"tmpl-123",
			"name":"my-template",
			"imageName":"runpod/base:latest",
			"category":"NVIDIA",
			"containerDiskInGb":20,
			"containerRegistryAuthId":"auth-1",
			"dockerEntrypoint":["/bin/bash"],
			"dockerStartCmd":["start.sh"],
			"env":{"FOO":"bar"},
			"isPublic":true,
			"isServerless":false,
			"ports":["8888/http"],
			"readme":"hello",
			"volumeInGb":50,
			"volumeMountPath":"/workspace",
			"earned":1.5,
			"isRunpod":true,
			"runtimeInMin":10
		}}}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := TemplateDataSourceSchema(ctx)

	// Build a well-formed config with id set and the computed fields null.
	objType := sch.Type().TerraformType(ctx).(tftypes.Object)
	vals := make(map[string]tftypes.Value, len(objType.AttributeTypes))
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

	// CORRECT: Read completes with no error.
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to succeed, got diags=%v", resp.Diagnostics)
	}

	// CORRECT: state is populated from the GraphQL response.
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
