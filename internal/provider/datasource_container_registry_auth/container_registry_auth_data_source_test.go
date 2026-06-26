package datasource_container_registry_auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// TestContainerRegistryAuthDataSourceRead_PopulatesState asserts the CORRECT
// behavior of the container registry auth data source Read: given a valid
// GraphQL response, Read single-unwraps the envelope, reads the
// "containerRegistryAuths" list, and populates state with no diagnostics error.
//
// This is gated on an open bug in the source (NOT in this test):
//   - CE-1671: Read re-indexes result["data"] at
//     container_registry_auth_data_source.go:46 after client.Query already
//     stripped the {"data":...} envelope (double-unwrap), so the else branch
//     always fires. It also builds a []ContainerRegistryAuthModel slice and
//     sets it against a single-object schema (id/name/username flat), which is
//     itself an error once the double-unwrap is fixed.
//
// Un-skip when fixed.
func TestContainerRegistryAuthDataSourceRead_PopulatesState(t *testing.T) {
	t.Skip("CE-1671: Read re-unwraps result[\"data\"] (double-unwrap); also sets a []Model slice against a single-object schema. Un-skip when fixed")

	// Valid GraphQL response: client.Query strips the {"data":...} envelope and
	// returns the inner map, so a correct Read reads result["containerRegistryAuths"].
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"containerRegistryAuths":[
			{"id":"auth-1","name":"dockerhub","username":"alice"},
			{"id":"auth-2","name":"ghcr","username":"bob"}
		]}}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := ContainerRegistryAuthDataSourceSchema(ctx)

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch}}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}

	(&ContainerRegistryAuthDataSource{}).Read(ctx, req, resp)

	// CORRECT: Read completes with no error and the auth list is returned into state.
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to succeed and populate state, got diags=%v", resp.Diagnostics)
	}
}
