package resource_container_registry_auth

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

// TestContainerRegistryAuthResource_Create verifies that Create POSTs the
// name/username/password to /v2/registries and populates state from the
// JSON response (id, name, username).
func TestContainerRegistryAuthResource_Create(t *testing.T) {
	ctx := context.Background()
	sch := ContainerRegistryAuthResourceSchema(ctx)

	m := ContainerRegistryAuthModel{
		Id:       types.StringNull(),
		Name:     types.StringValue("my-registry"),
		Password: types.StringValue("s3cret"),
		Username: types.StringValue("alice"),
	}

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config state: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cra-1","name":"my-registry","username":"alice"}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&ContainerRegistryAuthResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics: %v", resp.Diagnostics.Errors())
	}

	if gotMethod != http.MethodPost {
		t.Errorf("request method = %q, want POST", gotMethod)
	}
	if gotPath != "/v2/registries" {
		t.Errorf("request path = %q, want /v2/registries", gotPath)
	}
	if gotAuth != "Bearer testkey123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer testkey123")
	}
	if gotBody["name"] != "my-registry" || gotBody["username"] != "alice" || gotBody["password"] != "s3cret" {
		t.Errorf("request body = %v, want name/username/password populated", gotBody)
	}

	var out ContainerRegistryAuthModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if out.Id.ValueString() != "cra-1" {
		t.Errorf("state Id = %q, want cra-1", out.Id.ValueString())
	}
	if out.Name.ValueString() != "my-registry" {
		t.Errorf("state Name = %q, want my-registry", out.Name.ValueString())
	}
	if out.Username.ValueString() != "alice" {
		t.Errorf("state Username = %q, want alice", out.Username.ValueString())
	}
}

// TestContainerRegistryAuthResource_Create_Accepts201 locks in CE-1681: POST
// /v2/registries returns 201 Created, so Create must treat 201 as success.
func TestContainerRegistryAuthResource_Create_Accepts201(t *testing.T) {
	ctx := context.Background()
	sch := ContainerRegistryAuthResourceSchema(ctx)
	m := ContainerRegistryAuthModel{
		Id:       types.StringNull(),
		Name:     types.StringValue("my-registry"),
		Password: types.StringValue("s3cret"),
		Username: types.StringValue("alice"),
	}
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"cra-1","name":"my-registry","username":"alice"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&ContainerRegistryAuthResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create must accept HTTP 201 (CE-1681): %v", resp.Diagnostics.Errors())
	}
	var out ContainerRegistryAuthModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.Id.ValueString() != "cra-1" {
		t.Errorf("id = %q, want cra-1 (Create must set id on a 201 response)", out.Id.ValueString())
	}
}

// TestContainerRegistryAuthResource_Create_PartialResponse_ReturnsDiagnostic
// asserts the CORRECT behavior: when the API returns a 200 body containing
// "id" but omitting "name"/"username", Create should return a diagnostics
// error gracefully rather than panicking on an unchecked type assertion.
func TestContainerRegistryAuthResource_Create_PartialResponse_ReturnsDiagnostic(t *testing.T) {

	ctx := context.Background()
	sch := ContainerRegistryAuthResourceSchema(ctx)

	m := ContainerRegistryAuthModel{
		Id:       types.StringNull(),
		Name:     types.StringValue("my-registry"),
		Password: types.StringValue("s3cret"),
		Username: types.StringValue("alice"),
	}
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config state: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	// Response includes "id" but omits "name" and "username".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cra-1"}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&ContainerRegistryAuthResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)

	// CORRECT behavior: a partial response surfaces as a diagnostics error,
	// not a panic.
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected diagnostics error for partial response missing name/username, got none")
	}
}

// TestContainerRegistryAuthResource_Read verifies that Read GETs
// /v2/registries/{id} and updates name/username from the response.
func TestContainerRegistryAuthResource_Read(t *testing.T) {
	ctx := context.Background()
	sch := ContainerRegistryAuthResourceSchema(ctx)

	m := ContainerRegistryAuthModel{
		Id:       types.StringValue("cra-1"),
		Name:     types.StringValue("old-name"),
		Password: types.StringValue("s3cret"),
		Username: types.StringValue("old-user"),
	}
	state := tfsdk.State{Schema: sch}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatalf("build read state: %v", d)
	}

	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cra-1","name":"new-name","username":"new-user"}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&ContainerRegistryAuthResource{}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read returned diagnostics: %v", resp.Diagnostics.Errors())
	}

	if gotMethod != http.MethodGet {
		t.Errorf("request method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/registries/cra-1" {
		t.Errorf("request path = %q, want /v2/registries/cra-1", gotPath)
	}

	var out ContainerRegistryAuthModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if out.Name.ValueString() != "new-name" {
		t.Errorf("state Name = %q, want new-name", out.Name.ValueString())
	}
	if out.Username.ValueString() != "new-user" {
		t.Errorf("state Username = %q, want new-user", out.Username.ValueString())
	}
}

// TestContainerRegistryAuthResource_Delete verifies that Delete issues a DELETE
// to /v2/registries/{id} and treats a 204 response as success.
func TestContainerRegistryAuthResource_Delete(t *testing.T) {
	ctx := context.Background()
	sch := ContainerRegistryAuthResourceSchema(ctx)

	m := ContainerRegistryAuthModel{
		Id:       types.StringValue("cra-1"),
		Name:     types.StringValue("my-registry"),
		Password: types.StringValue("s3cret"),
		Username: types.StringValue("alice"),
	}
	state := tfsdk.State{Schema: sch}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatalf("build delete state: %v", d)
	}

	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch}}
	(&ContainerRegistryAuthResource{}).Delete(ctx, resource.DeleteRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete returned diagnostics: %v", resp.Diagnostics.Errors())
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("request method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/v2/registries/cra-1" {
		t.Errorf("request path = %q, want /v2/registries/cra-1", gotPath)
	}
}
