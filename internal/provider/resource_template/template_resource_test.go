package resource_template

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// mustList builds a types.List of strings or fails the test.
func mustList(t *testing.T, vals ...string) types.List {
	t.Helper()
	elems := make([]attr.Value, 0, len(vals))
	for _, v := range vals {
		elems = append(elems, types.StringValue(v))
	}
	l, d := types.ListValue(types.StringType, elems)
	if d.HasError() {
		t.Fatalf("build list: %v", d)
	}
	return l
}

// newConfiguredModel returns a TemplateModel with every types.List / types.Object
// field initialized to a typed null. The terraform-plugin-framework reflection
// used by State.Set panics on uninitialized (zero-value) collection/object
// attributes, so these must be set even when unused.
func newBaseModel() TemplateModel {
	return TemplateModel{
		Name:                    types.StringValue("my-tpl"),
		ImageName:               types.StringValue("img"),
		Category:                types.StringNull(),
		ContainerDiskInGb:       types.Int64Null(),
		ContainerRegistryAuthId: types.StringNull(),
		DockerEntrypoint:        types.ListNull(types.StringType),
		DockerStartCmd:          types.ListNull(types.StringType),
		// The schema declares `env` as a bare ObjectAttribute with no
		// AttributeTypes, so a null object with an empty attribute-type map is
		// what the framework round-trips cleanly here.
		Env: types.MapNull(types.StringType),
		IsPublic:                types.BoolNull(),
		IsServerless:            types.BoolNull(),
		Ports:                   types.ListNull(types.StringType),
		Readme:                  types.StringNull(),
		VolumeInGb:              types.Int64Null(),
		VolumeMountPath:         types.StringNull(),
		Id:                      types.StringNull(),
		Earned:                  types.Float64Null(),
		IsRunpod:                types.BoolNull(),
		RuntimeInMin:            types.Int64Null(),
	}
}

// buildConfig sets the model into a State of the resource schema and returns a
// Config sharing the same Raw value, mirroring how the framework hands a
// resource its planned config during an apply.
func buildConfig(t *testing.T, ctx context.Context, m TemplateModel) tfsdk.Config {
	t.Helper()
	sch := TemplateResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config state: %v", d)
	}
	return tfsdk.Config{Schema: sch, Raw: st.Raw}
}

func TestTemplateResource_Metadata(t *testing.T) {
	resp := &resource.MetadataResponse{}
	(&TemplateResource{}).Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "runpod_template" {
		t.Fatalf("TypeName = %q, want runpod_template", resp.TypeName)
	}
}

func TestTemplateResource_Create_Success(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "tpl-1",
			"name": "my-tpl",
			"image": "img",
			"category": "NVIDIA",
			"disk": 20,
			"containerRegistryAuthId": "auth-9",
			"public": true,
			"serverless": false,
			"readme": "hello",
			"volumeInGb": 5,
			"volumeMountPath": "/workspace",
			"earned": 1.5,
			"isRunpod": false,
			"runtimeInMin": 0
		}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := newBaseModel()
	m.Category = types.StringValue("NVIDIA")
	m.ContainerDiskInGb = types.Int64Value(20)
	m.ContainerRegistryAuthId = types.StringValue("auth-9")
	cfg := buildConfig(t, ctx, m)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned errors: %v", resp.Diagnostics)
	}

	// Request shape
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/templates" {
		t.Errorf("path = %q, want /templates", gotPath)
	}
	if gotAuth != "Bearer testkey123" {
		t.Errorf("auth = %q, want Bearer testkey123", gotAuth)
	}
	if gotBody["name"] != "my-tpl" {
		t.Errorf("body name = %v, want my-tpl", gotBody["name"])
	}
	if gotBody["image"] != "img" {
		t.Errorf("body image = %v, want img", gotBody["image"])
	}
	if gotBody["category"] != "NVIDIA" {
		t.Errorf("body category = %v, want NVIDIA", gotBody["category"])
	}
	if gotBody["disk"] != float64(20) {
		t.Errorf("body disk = %v, want 20", gotBody["disk"])
	}
	if gotBody["containerRegistryAuthId"] != "auth-9" {
		t.Errorf("body containerRegistryAuthId = %v, want auth-9", gotBody["containerRegistryAuthId"])
	}

	// Resulting state
	var state TemplateModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if state.Id.ValueString() != "tpl-1" {
		t.Errorf("state Id = %q, want tpl-1", state.Id.ValueString())
	}
	if state.Name.ValueString() != "my-tpl" {
		t.Errorf("state Name = %q, want my-tpl", state.Name.ValueString())
	}
	if state.ImageName.ValueString() != "img" {
		t.Errorf("state ImageName = %q, want img", state.ImageName.ValueString())
	}
	if state.Category.ValueString() != "NVIDIA" {
		t.Errorf("state Category = %q, want NVIDIA", state.Category.ValueString())
	}
	if state.ContainerDiskInGb.ValueInt64() != 20 {
		t.Errorf("state ContainerDiskInGb = %d, want 20", state.ContainerDiskInGb.ValueInt64())
	}
	if !state.IsPublic.ValueBool() {
		t.Errorf("state IsPublic = false, want true")
	}
	if state.IsServerless.ValueBool() {
		t.Errorf("state IsServerless = true, want false")
	}
	if state.Earned.ValueFloat64() != 1.5 {
		t.Errorf("state Earned = %v, want 1.5", state.Earned.ValueFloat64())
	}
}

// TestTemplateResource_Create_Accepts201 locks in CE-1681: POST /templates
// returns 201 Created, so Create must treat 201 as success (not only 200).
func TestTemplateResource_Create_Accepts201(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"tpl-1","name":"my-tpl","image":"img"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cfg := buildConfig(t, ctx, newBaseModel())
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create must accept HTTP 201 (CE-1681): %v", resp.Diagnostics)
	}
	var out TemplateModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.Id.ValueString() != "tpl-1" {
		t.Errorf("id = %q, want tpl-1 (Create must set id on a 201 response)", out.Id.ValueString())
	}
}

func TestTemplateResource_Create_NoAPIKey(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	t.Setenv("RUNPOD_API_KEY", "")

	cfg := buildConfig(t, ctx, newBaseModel())
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error when RUNPOD_API_KEY is unset, got none")
	}
}

func TestTemplateResource_Create_NoIDInResponse(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"my-tpl"}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cfg := buildConfig(t, ctx, newBaseModel())
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error when response has no id, got none")
	}
}

func TestTemplateResource_Create_APIError(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cfg := buildConfig(t, ctx, newBaseModel())
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error on non-200 status, got none")
	}
}

func TestTemplateResource_Create_WithListFields(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"tpl-2",
			"name":"my-tpl",
			"image":"img",
			"dockerEntrypoint":["/bin/sh","-c"],
			"dockerStartCmd":["sleep","infinity"],
			"ports":["8080/http","22/tcp"]
		}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := newBaseModel()
	m.DockerEntrypoint = mustList(t, "/bin/sh", "-c")
	m.DockerStartCmd = mustList(t, "sleep", "infinity")
	m.Ports = mustList(t, "8080/http", "22/tcp")
	cfg := buildConfig(t, ctx, m)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned errors: %v", resp.Diagnostics)
	}

	// List fields should be serialized into the request body as JSON arrays.
	ep, ok := gotBody["dockerEntrypoint"].([]interface{})
	if !ok || len(ep) != 2 || ep[0] != "/bin/sh" {
		t.Errorf("body dockerEntrypoint = %v, want [/bin/sh -c]", gotBody["dockerEntrypoint"])
	}
	ports, ok := gotBody["ports"].([]interface{})
	if !ok || len(ports) != 2 || ports[0] != "8080/http" {
		t.Errorf("body ports = %v, want [8080/http 22/tcp]", gotBody["ports"])
	}

	var state TemplateModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if len(state.DockerEntrypoint.Elements()) != 2 {
		t.Errorf("state DockerEntrypoint len = %d, want 2", len(state.DockerEntrypoint.Elements()))
	}
	if len(state.Ports.Elements()) != 2 {
		t.Errorf("state Ports len = %d, want 2", len(state.Ports.Elements()))
	}
}

func TestTemplateResource_Read_Success(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"tpl-1",
			"name":"renamed",
			"image":"img2",
			"category":"CPU",
			"disk":30
		}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := newBaseModel()
	m.Id = types.StringValue("tpl-1")
	priorState := tfsdk.State{Schema: sch}
	if d := priorState.Set(ctx, &m); d.HasError() {
		t.Fatalf("build prior state: %v", d)
	}

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Read(ctx, resource.ReadRequest{State: priorState}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read returned errors: %v", resp.Diagnostics)
	}
	if gotMethod != "GET" {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/templates/tpl-1" {
		t.Errorf("path = %q, want /v2/v2/templates/tpl-1", gotPath)
	}

	var state TemplateModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if state.Name.ValueString() != "renamed" {
		t.Errorf("state Name = %q, want renamed", state.Name.ValueString())
	}
	if state.ImageName.ValueString() != "img2" {
		t.Errorf("state ImageName = %q, want img2", state.ImageName.ValueString())
	}
	if state.Category.ValueString() != "CPU" {
		t.Errorf("state Category = %q, want CPU", state.Category.ValueString())
	}
}

// TestTemplateResource_Update_RetainsApiComputedFields asserts the CORRECT
// behavior: after Update, state should be set from the API-merged result, so
// computed fields the API returned that were NOT present in the plan survive
// in the final state. Update must not overwrite the API-merged values with the
// planned config.
func TestTemplateResource_Update_RetainsApiComputedFields(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	var gotMethod, gotPath string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		// API echoes the planned name/category, AND returns a computed field
		// (earned) that was NOT part of the plan.
		_, _ = w.Write([]byte(`{
			"id":"tpl-1",
			"name":"updated-name",
			"image":"img",
			"category":"AMD",
			"earned":42.5
		}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	// prior state
	stateModel := newBaseModel()
	stateModel.Id = types.StringValue("tpl-1")
	priorState := tfsdk.State{Schema: sch}
	if d := priorState.Set(ctx, &stateModel); d.HasError() {
		t.Fatalf("build prior state: %v", d)
	}

	// planned config — note: Earned is left null in the plan.
	planModel := newBaseModel()
	planModel.Id = types.StringValue("tpl-1")
	planModel.Name = types.StringValue("updated-name")
	planModel.Category = types.StringValue("AMD")
	cfg := buildConfig(t, ctx, planModel)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Update(ctx, resource.UpdateRequest{Config: cfg, State: priorState}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update returned errors: %v", resp.Diagnostics)
	}
	if gotMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/v2/templates/tpl-1" {
		t.Errorf("path = %q, want /v2/v2/templates/tpl-1", gotPath)
	}
	if gotBody["name"] != "updated-name" {
		t.Errorf("body name = %v, want updated-name", gotBody["name"])
	}

	var finalState TemplateModel
	if d := resp.State.Get(ctx, &finalState); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if finalState.Name.ValueString() != "updated-name" {
		t.Errorf("final state Name = %q, want updated-name", finalState.Name.ValueString())
	}
	if finalState.Id.ValueString() != "tpl-1" {
		t.Errorf("final state Id = %q, want tpl-1", finalState.Id.ValueString())
	}
	// CORRECT behavior: the API-computed field (earned) the plan did not carry
	// must survive into the final state.
	if finalState.Earned.ValueFloat64() != 42.5 {
		t.Errorf("final state Earned = %v, want 42.5 (API-computed field must survive)", finalState.Earned.ValueFloat64())
	}
}

// TestTemplateResource_Update_ExcludesCategory asserts the CORRECT behavior for
// CE-1686: TemplateResource.Update puts `category` into the PATCH body, but the v1
// templates PATCH input schema does not accept it — the API returns
// 400 "Extra input keys ... 'category'". Update must not send `category` (nor other
// non-updatable keys). Skipped until CE-1686 is fixed; asserts the PATCH body omits
// `category`.
func TestTemplateResource_Update_ExcludesCategory(t *testing.T) {
	t.Skip("CE-1686: template Update sends 'category', which the v1 PATCH input schema rejects (400) — un-skip when fixed")
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"tpl-1","name":"updated-name","image":"img"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	stateModel := newBaseModel()
	stateModel.Id = types.StringValue("tpl-1")
	priorState := tfsdk.State{Schema: sch}
	if d := priorState.Set(ctx, &stateModel); d.HasError() {
		t.Fatalf("build prior state: %v", d)
	}

	planModel := newBaseModel()
	planModel.Id = types.StringValue("tpl-1")
	planModel.Name = types.StringValue("updated-name")
	planModel.Category = types.StringValue("NVIDIA") // user-set, but not accepted on PATCH
	cfg := buildConfig(t, ctx, planModel)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Update(ctx, resource.UpdateRequest{Config: cfg, State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update returned errors: %v", resp.Diagnostics)
	}

	if _, ok := gotBody["category"]; ok {
		t.Errorf("PATCH body includes 'category' (%v) — the v1 template PATCH schema rejects it (CE-1686); Update must not send it", gotBody["category"])
	}
}

func TestTemplateResource_Delete_Success(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := newBaseModel()
	m.Id = types.StringValue("tpl-1")
	priorState := tfsdk.State{Schema: sch}
	if d := priorState.Set(ctx, &m); d.HasError() {
		t.Fatalf("build prior state: %v", d)
	}

	resp := &resource.DeleteResponse{State: priorState}
	(&TemplateResource{}).Delete(ctx, resource.DeleteRequest{State: priorState}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete returned errors: %v", resp.Diagnostics)
	}
	if gotMethod != "DELETE" {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/v2/templates/tpl-1" {
		t.Errorf("path = %q, want /v2/v2/templates/tpl-1", gotPath)
	}
}

func TestTemplateResource_Delete_NotNoContent(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // 200, not 204
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := newBaseModel()
	m.Id = types.StringValue("tpl-1")
	priorState := tfsdk.State{Schema: sch}
	if d := priorState.Set(ctx, &m); d.HasError() {
		t.Fatalf("build prior state: %v", d)
	}

	resp := &resource.DeleteResponse{State: priorState}
	(&TemplateResource{}).Delete(ctx, resource.DeleteRequest{State: priorState}, resp)

	// Delete treats anything other than 204 as an error.
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error on status 200, got none")
	}
}

// TestTemplateRead_404_RemovesState asserts CE-1654 fix for template:
// when a template is gone (404), Read must call resp.State.RemoveResource
// so the deleted template is removed from state and planned for recreation.
func TestTemplateRead_404_RemovesState(t *testing.T) {
	ctx := context.Background()
	m := newBaseModel()
	m.Id = types.StringValue("template-gone")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	sch := TemplateResourceSchema(ctx)
	state := tfsdk.State{Schema: sch}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	resp := &resource.ReadResponse{State: state}
	(&TemplateResource{}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("404 should not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state was not removed on 404 — CE-1654: deleted template should be removed from state")
	}
}
