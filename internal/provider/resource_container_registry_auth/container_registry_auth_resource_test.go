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
// name/username/password to /containerregistryauth and populates state from the
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
	if gotPath != "/containerregistryauth" {
		t.Errorf("request path = %q, want /containerregistryauth", gotPath)
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

// TestContainerRegistryAuthResource_Create_MissingNamePanics documents a bug:
// Create reads result["name"].(string) and result["username"].(string) with
// unchecked type assertions (resource.go:101-102). When the API returns a 200
// body containing "id" but omitting "name"/"username", these assertions panic
// instead of producing a clean diagnostic. This test characterizes that actual
// behavior; it does NOT fix the source.
func TestContainerRegistryAuthResource_Create_MissingNamePanics(t *testing.T) {
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

	// Response includes "id" (so the ok-checked branch succeeds) but omits
	// "name" and "username", triggering the unchecked assertions.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cra-1"}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	defer func() {
		if rec := recover(); rec != nil {
			// BUG (container_registry_auth_resource.go:101-102): unchecked
			// type assertion result["name"].(string) panics on missing field.
			t.Logf("CONFIRMED BUG: Create panicked on missing response field: %v", rec)
		} else {
			t.Errorf("expected panic from unchecked result[\"name\"].(string) on missing field; got none — source may have been fixed")
		}
	}()

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&ContainerRegistryAuthResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, resp)
}

// TestContainerRegistryAuthResource_Read verifies that Read GETs
// /containerregistryauth/{id} and updates name/username from the response.
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
	if gotPath != "/containerregistryauth/cra-1" {
		t.Errorf("request path = %q, want /containerregistryauth/cra-1", gotPath)
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
// to /containerregistryauth/{id} and treats a 204 response as success.
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
	if gotPath != "/containerregistryauth/cra-1" {
		t.Errorf("request path = %q, want /containerregistryauth/cra-1", gotPath)
	}
}
