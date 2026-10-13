package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

func TestFlattenTemplateDetail(t *testing.T) {
	ctx := context.Background()
	registry := "reg_abc"

	template := sdktypes.Template{
		Id:                  "tpl_abc",
		Name:                "My PyTorch Template",
		Image:               "runpod/pytorch:2.8.0",
		Category:            sdktypes.TemplateCategoryNVIDIA,
		Disk:                50,
		Args:                "--verbose",
		Env:                 map[string]string{"JUPYTER_PASSWORD": "changeme"},
		Ports:               []string{"8888/http", "22/tcp"},
		Public:              true,
		Serverless:          false,
		Registry:            &registry,
		StartJupyter:        true,
		StartSsh:            true,
		AllowedCudaVersions: []string{"12.8", "12.9"},
		Mounts: sdktypes.TemplateMounts{
			Persistent: &sdktypes.PersistentMount{Path: "/workspace", Size: 20},
		},
	}

	model, diags := flattenTemplateDetail(ctx, template)
	if diags.HasError() {
		t.Fatal(diags)
	}

	if model.Id.ValueString() != "tpl_abc" || model.Name.ValueString() != "My PyTorch Template" {
		t.Fatalf("id/name: %+v", model)
	}
	if model.Category.ValueString() != "NVIDIA" || model.Disk.ValueInt64() != 50 {
		t.Fatalf("category/disk: %+v", model)
	}
	if !model.Public.ValueBool() || model.Serverless.ValueBool() {
		t.Fatalf("public/serverless: %+v", model)
	}
	if model.Registry.ValueString() != "reg_abc" || !model.StartJupyter.ValueBool() || !model.StartSsh.ValueBool() {
		t.Fatalf("registry/start flags: %+v", model)
	}
	if got := len(model.AllowedCudaVersions.Elements()); got != 2 {
		t.Fatalf("allowed_cuda_versions has %d entries, want 2", got)
	}
	if got := len(model.Env.Elements()); got != 1 {
		t.Fatalf("env has %d entries, want 1", got)
	}
	if model.Mounts == nil || model.Mounts.Persistent == nil ||
		model.Mounts.Persistent.Path.ValueString() != "/workspace" ||
		model.Mounts.Persistent.Size.ValueInt64() != 20 {
		t.Fatalf("mounts: %+v", model.Mounts)
	}

	assertDataSourceModelFits(t, NewTemplateDataSource, templateDataSourceState(model))
}

func TestFlattenTemplateDetailWithoutMountOrRegistry(t *testing.T) {
	ctx := context.Background()

	model, diags := flattenTemplateDetail(ctx, sdktypes.Template{
		Id:                  "tpl_bare",
		Name:                "bare",
		Image:               "runpod/base:1.0.0",
		Category:            sdktypes.TemplateCategoryCPU,
		Disk:                10,
		Env:                 map[string]string{},
		Ports:               []string{},
		AllowedCudaVersions: []string{},
	})
	if diags.HasError() {
		t.Fatal(diags)
	}

	if !model.Registry.IsNull() {
		t.Fatalf("registry = %v, want null", model.Registry)
	}
	if model.Mounts == nil || model.Mounts.Persistent != nil {
		t.Fatalf("mounts: %+v", model.Mounts)
	}

	assertDataSourceModelFits(t, NewTemplateDataSource, templateDataSourceState(model))
}

func TestTemplateDataSourceStateStartsTheShimAttributesNull(t *testing.T) {
	// docker_entrypoint is filled from the v2 template and the four REST v1
	// read-backs after it; until then they are null, which is what a
	// configuration sees of the four when the legacy host is gone.
	state := templateDataSourceState(templateDetailModel{
		Id:    types.StringValue("tpl_abc"),
		Name:  types.StringValue("trainer"),
		Image: types.StringValue("runpod/pytorch:1.0.2"),
	})

	if state.Id.ValueString() != "tpl_abc" || state.Name.ValueString() != "trainer" ||
		state.Image.ValueString() != "runpod/pytorch:1.0.2" {
		t.Errorf("shared fields did not carry over: %+v", state)
	}
	if !state.DockerEntrypoint.IsNull() || !state.Readme.IsNull() || !state.Earned.IsNull() ||
		!state.IsRunpod.IsNull() || !state.RuntimeInMin.IsNull() {
		t.Errorf("shim attributes should start null: %+v", state)
	}
	// The null list still has to carry its element type, or the state cannot
	// hold it.
	if state.DockerEntrypoint.ElementType(context.Background()) != types.StringType {
		t.Errorf("docker_entrypoint = %v, want a null list of strings", state.DockerEntrypoint)
	}
}

func TestTemplateDataSourceReadsTheEntrypointFromV2(t *testing.T) {
	ctx := context.Background()

	// The REST v1 route answers a different entrypoint on purpose: the value
	// in state has to be the v2 template's, while readme still comes from v1.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/templates/tpl_abc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"tpl_abc","name":"trainer","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,` +
				`"args":"{\"entrypoint\":[\"/bin/bash\",\"-c\"],\"cmd\":[\"python -u main.py\"]}",` +
				`"entrypoint":["/bin/bash","-c"],"cmd":["python -u main.py"],"env":{},"ports":[],"allowedCudaVersions":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/templates/tpl_abc":
			_, _ = w.Write([]byte(`{"dockerEntrypoint":["/bin/sh"],"readme":"# trainer"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	d := &templateDataSource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	var state templateDataSourceModel
	readDataSource(ctx, t, d, map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, "tpl_abc")}, &state)

	var entrypoint []string
	if diags := state.DockerEntrypoint.ElementsAs(ctx, &entrypoint, false); diags.HasError() {
		t.Fatal(diags)
	}
	if !slices.Equal(entrypoint, []string{"/bin/bash", "-c"}) {
		t.Errorf("docker_entrypoint = %v, want the v2 template's [/bin/bash -c]", entrypoint)
	}
	if state.Readme.ValueString() != "# trainer" {
		t.Errorf("readme = %v, want the REST v1 value", state.Readme)
	}
}

func TestTemplateDataSourceReadsTheRestV1Fields(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"dockerEntrypoint":["/bin/bash","-c"],"readme":"# trainer","earned":1.5,"isRunpod":true,"runtimeInMin":120}`))
	}))
	defer srv.Close()

	d := &templateDataSource{legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL})}
	state := templateDataSourceState(templateDetailModel{Id: types.StringValue("tpl_abc")})

	diags := d.readLegacyTemplateFields(context.Background(), &state)
	if diags.HasError() {
		t.Fatal(diags)
	}

	if gotPath != "/templates/tpl_abc" {
		t.Errorf("path = %q, want /templates/tpl_abc", gotPath)
	}
	// docker_entrypoint comes from the v2 template, so the REST v1 value the
	// response carries must not reach it.
	if !state.DockerEntrypoint.IsNull() {
		t.Errorf("docker_entrypoint = %v, want it left alone by the REST v1 read", state.DockerEntrypoint)
	}
	if state.Readme.ValueString() != "# trainer" || state.Earned.ValueFloat64() != 1.5 ||
		!state.IsRunpod.ValueBool() || state.RuntimeInMin.ValueInt64() != 120 {
		t.Errorf("shim attributes: %+v", state)
	}
}

func TestTemplateDataSourceWarnsWhenTheRestV1HostFails(t *testing.T) {
	// Nothing on this data source is configurable, so no configuration depends
	// on the legacy host: the read warns and the four shim attributes stay null,
	// while the docker_entrypoint the v2 read filled is kept.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	ctx := context.Background()
	d := &templateDataSource{legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL})}
	state := templateDataSourceState(templateDetailModel{Id: types.StringValue("tpl_abc")})
	state.Readme = types.StringValue("stale")
	entrypoint, diags := types.ListValueFrom(ctx, types.StringType, []string{"/bin/bash"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	state.DockerEntrypoint = entrypoint

	diags = d.readLegacyTemplateFields(ctx, &state)
	if diags.HasError() {
		t.Fatalf("a failed shim read must not break the data source: %v", diags)
	}
	if diags.WarningsCount() != 1 {
		t.Fatalf("warnings = %d, want 1: %v", diags.WarningsCount(), diags)
	}
	if !state.Readme.IsNull() || !state.Earned.IsNull() ||
		!state.IsRunpod.IsNull() || !state.RuntimeInMin.IsNull() {
		t.Errorf("shim attributes should be null after a failed read: %+v", state)
	}
	if len(state.DockerEntrypoint.Elements()) != 1 {
		t.Errorf("docker_entrypoint = %v, want the v2 value kept", state.DockerEntrypoint)
	}
}

func TestTemplateDataSourceDeprecatesTheShimAttributes(t *testing.T) {
	ctx := context.Background()
	var schemaResp datasource.SchemaResponse
	NewTemplateDataSource().Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}

	for _, name := range []string{"readme", "earned", "is_runpod", "runtime_in_min"} {
		attribute, ok := schemaResp.Schema.Attributes[name]
		if !ok {
			t.Errorf("the schema has no %q attribute", name)
			continue
		}
		if attribute.GetDeprecationMessage() != legacyShimDeprecation {
			t.Errorf("%s: deprecation message = %q", name, attribute.GetDeprecationMessage())
		}
	}

	// docker_entrypoint is read from REST v2, so it is a plain attribute.
	entrypoint, ok := schemaResp.Schema.Attributes["docker_entrypoint"]
	if !ok {
		t.Fatal(`the schema has no "docker_entrypoint" attribute`)
	}
	if entrypoint.GetDeprecationMessage() != "" || strings.Contains(entrypoint.GetDescription(), legacyShimDescriptionSuffix) {
		t.Errorf("docker_entrypoint must not be marked as a shim: %q / %q",
			entrypoint.GetDeprecationMessage(), entrypoint.GetDescription())
	}
}
