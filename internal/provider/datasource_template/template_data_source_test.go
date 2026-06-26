package datasource_template

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestTemplateModel_MalformedStructTags_CE1652Adjacent characterizes a hard
// blocker in this NEW data source that sits in front of the CE-1652 double-unwrap.
//
// The generated TemplateModel (template_data_source_gen.go:117-130) has MALFORMED
// `tfsdk` struct tags — every field after Name is missing its closing quote, e.g.
// `tfsdk:"container_disk_in_gb` instead of `tfsdk:"container_disk_in_gb"`. As a
// result the terraform-plugin-framework cannot reflect over TemplateModel at all:
// any State/Config Get or Set using TemplateModel errors with
// "need a struct tag for \"tfsdk\" on ContainerDiskInGb".
//
// This means the very FIRST line of Read — req.Config.Get(ctx, &config) at
// template_data_source.go:28 — fails before the GraphQL query is ever issued.
// The downstream CE-1652 double-unwrap (line 71) is therefore currently dead code.
func TestTemplateModel_MalformedStructTags_CE1652Adjacent(t *testing.T) {
	ctx := context.Background()
	sch := TemplateDataSourceSchema(ctx)

	// Attempting to round-trip TemplateModel through the framework fails on the
	// broken tags, independent of any network call.
	st := tfsdk.State{Schema: sch}
	diags := st.Set(ctx, &TemplateModel{Id: types.StringValue("tmpl-123")})
	if !diags.HasError() {
		t.Fatalf("expected struct-tag reflection error from malformed TemplateModel tags, got none")
	}
	found := false
	for _, d := range diags.Errors() {
		if strings.Contains(d.Detail(), `need a struct tag for "tfsdk"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected detail mentioning missing tfsdk struct tag; got %v", diags)
	}
}

// TestTemplateDataSourceRead_ConfigDecodeFails_CE1652 characterizes the real
// current behavior when Read is invoked with a VALID config and a VALID GraphQL
// response. This NEW data source carries two stacked defects:
//
//  1. The generated TemplateModel has malformed `tfsdk` struct tags
//     (template_data_source_gen.go:117-130), so req.Config.Get(ctx, &config) at
//     template_data_source.go:28 fails with the reflection error
//     "need a struct tag for \"tfsdk\"". Read returns at line 31 before any query.
//
//  2. Behind that, the CE-1652 double-unwrap at template_data_source.go:71
//     (result["data"] re-indexed after client.Query already stripped the envelope)
//     would also fail. It is unreachable today because (1) short-circuits first.
//
// Here we build the config Raw directly via tftypes (NOT via TemplateModel, which
// can't be reflected) so the config itself is well-formed; Read still fails — and
// the failing point is the struct-tag decode at line 28, NOT the network/unwrap.
func TestTemplateDataSourceRead_ConfigDecodeFails_CE1652(t *testing.T) {
	// A fully valid GraphQL response for the template query. With the bugs, none of
	// this is ever consumed — Read returns before issuing the query.
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

	// Build a well-formed config Raw directly from the schema's tftypes object type,
	// bypassing the un-reflectable TemplateModel. id is set; computed fields are null.
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

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to error, got none; diags=%v", resp.Diagnostics)
	}

	// Read fails at req.Config.Get (template_data_source.go:28) on the malformed
	// struct tags — NOT at the double-unwrap. Confirm the actual failing point.
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), `need a struct tag for "tfsdk"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the struct-tag decode error at Read line 28; got diags=%v", resp.Diagnostics)
	}
}
