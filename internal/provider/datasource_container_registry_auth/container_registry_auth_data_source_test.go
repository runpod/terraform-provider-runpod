package datasource_container_registry_auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// TestContainerRegistryAuthDataSourceRead_PopulatesState tests the REST migration:
// Uses v2 endpoint GET /v2/registries instead of GraphQL containerRegistryAuths query
func TestContainerRegistryAuthDataSourceRead_PopulatesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request is GET /registries (not GraphQL)
		if r.Method != "GET" {
			t.Errorf("expected GET method, got %q", r.Method)
		}
		if r.URL.Path != "/v2/registries" {
			t.Errorf("expected path /v2/registries, got %q", r.URL.Path)
		}
		// Verify Bearer token
		auth := r.Header.Get("Authorization")
		if auth != "Bearer testkey123" {
			t.Errorf("expected Bearer token, got %q", auth)
		}
		// Return v2 REST format: {data: {registries: [...]}}
		_, _ = w.Write([]byte(`{"data":{"registries":[
			{"id":"auth-1","name":"dockerhub","username":"alice","password":"secret123","createdAt":"2024-01-01T00:00:00Z","updatedAt":"2024-01-02T00:00:00Z"},
			{"id":"auth-2","name":"ghcr","username":"bob","password":"token456","createdAt":"2024-02-01T00:00:00Z","updatedAt":"2024-02-02T00:00:00Z"}
		]}}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: ContainerRegistryAuthDataSourceSchema(ctx)}}
	(&ContainerRegistryAuthDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to succeed, got diags=%v", resp.Diagnostics)
	}

	var state ContainerRegistryAuthDataSourceModel
	diags := resp.State.Get(ctx, &state)
	if diags.HasError() {
		t.Fatalf("expected to read state back, got diags=%v", diags)
	}
	if len(state.ContainerRegistryAuths) != 2 {
		t.Fatalf("expected 2 auths, got %d", len(state.ContainerRegistryAuths))
	}
	if state.ContainerRegistryAuths[0].Name != types.StringValue("dockerhub") {
		t.Errorf("first auth name: want %q, got %v", "dockerhub", state.ContainerRegistryAuths[0].Name)
	}
	if state.ContainerRegistryAuths[1].Name != types.StringValue("ghcr") {
		t.Errorf("second auth name: want %q, got %v", "ghcr", state.ContainerRegistryAuths[1].Name)
	}
	// Verify all fields are populated including password (sensitive) and timestamps
	if state.ContainerRegistryAuths[0].Password != types.StringValue("secret123") {
		t.Errorf("first auth password: want %q, got %v", "secret123", state.ContainerRegistryAuths[0].Password)
	}
	if state.ContainerRegistryAuths[0].CreatedAt != types.StringValue("2024-01-01T00:00:00Z") {
		t.Errorf("first auth created_at: want %q, got %v", "2024-01-01T00:00:00Z", state.ContainerRegistryAuths[0].CreatedAt)
	}
	if state.ContainerRegistryAuths[0].UpdatedAt != types.StringValue("2024-01-02T00:00:00Z") {
		t.Errorf("first auth updated_at: want %q, got %v", "2024-01-02T00:00:00Z", state.ContainerRegistryAuths[0].UpdatedAt)
	}
}
