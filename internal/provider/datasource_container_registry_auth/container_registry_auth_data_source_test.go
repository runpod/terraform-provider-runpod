package datasource_container_registry_auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// TestContainerRegistryAuthDataSourceRead_DoubleUnwrap_CE1652 characterizes the
// real current behavior of this NEW data source: it reintroduces the systemic
// CE-1652 double-unwrap bug.
//
// client.Query() (internal/provider/client/runpod_client.go:82-83) already strips
// the top-level {"data":...} GraphQL envelope and returns the INNER map. A correct
// caller would read result["containerRegistryAuths"] directly. Instead, the Read at
// internal/provider/datasource_container_registry_auth/container_registry_auth_data_source.go:46
// does result["data"].(map[string]interface{}) AGAIN, which never matches, so it
// falls into the else branch and always emits "Failed to get data from response"
// (container_registry_auth_data_source.go:67) regardless of a valid GraphQL response.
//
// NOTE: there is a SECOND latent bug downstream that this test cannot reach because
// the double-unwrap short-circuits first: the Read builds a []ContainerRegistryAuthModel
// slice (line 48) and calls resp.State.Set(ctx, &models) (line 58) against a
// SINGLE-OBJECT schema (id/name/username flat — see container_registry_auth_data_source_gen.go).
// Setting a slice into a single-object schema would itself error. That path is
// dead until the double-unwrap is fixed.
func TestContainerRegistryAuthDataSourceRead_DoubleUnwrap_CE1652(t *testing.T) {
	// A fully valid GraphQL response. With the bug, none of this is ever consumed.
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

	// Read takes no config input (no required attributes consumed before the query).
	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch}}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}

	(&ContainerRegistryAuthDataSource{}).Read(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error from the CE-1652 double-unwrap, got none; diags=%v", resp.Diagnostics)
	}

	// The else branch at container_registry_auth_data_source.go:67 is the
	// double-unwrap failure.
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "Failed to get data from response") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected detail %q (CE-1652 double-unwrap else branch); got diags=%v",
			"Failed to get data from response", resp.Diagnostics)
	}
}
