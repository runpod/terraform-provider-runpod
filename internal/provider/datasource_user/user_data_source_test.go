package datasource_user

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// graphqlStub serves a canned GraphQL response for any request.
func graphqlStub(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST to GraphQL endpoint, got %q", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer testkey123" {
			t.Errorf("expected Bearer token, got %q", auth)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)
}

// TestUserRead_PopulatesState tests the GraphQL `myself` query: there is no
// user endpoint in the v2 REST API, so account info comes from GraphQL.
func TestUserRead_PopulatesState(t *testing.T) {
	graphqlStub(t, `{
		"data": {
			"myself": {
				"id": "u1",
				"pubKey": "ssh-ed25519 AAAA test@runpod"
			}
		}
	}`)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: UserDataSourceSchema(ctx)}}
	(&UserDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics)
	}

	var model UserModel
	diags := resp.State.Get(ctx, &model)
	if diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if model.Id.ValueString() != "u1" {
		t.Errorf("expected id %q, got %q", "u1", model.Id.ValueString())
	}
	if model.PubKey.ValueString() != "ssh-ed25519 AAAA test@runpod" {
		t.Errorf("expected pubKey %q, got %q", "ssh-ed25519 AAAA test@runpod", model.PubKey.ValueString())
	}
}

// TestUserRead_MissingPubKey tests that a `myself` response without pubKey
// still populates state with a null pub_key.
func TestUserRead_MissingPubKey(t *testing.T) {
	graphqlStub(t, `{"data": {"myself": {"id": "u2"}}}`)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: UserDataSourceSchema(ctx)}}
	(&UserDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for minimal response, got: %v", resp.Diagnostics)
	}

	var model UserModel
	diags := resp.State.Get(ctx, &model)
	if diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if model.Id.ValueString() != "u2" {
		t.Errorf("expected id %q, got %q", "u2", model.Id.ValueString())
	}
	if !model.PubKey.IsNull() {
		t.Errorf("expected pub_key to be null when omitted, got %v", model.PubKey)
	}
}

// TestUserRead_MissingIdField_AddsDiagnostic tests that missing id field causes an error.
func TestUserRead_MissingIdField_AddsDiagnostic(t *testing.T) {
	graphqlStub(t, `{"data": {"myself": {"pubKey": "x"}}}`)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: UserDataSourceSchema(ctx)}}
	(&UserDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error when id is missing")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "API Error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an \"API Error\" diagnostic, got: %v", resp.Diagnostics)
	}
}

// TestUserRead_Error_AddsDiagnostic tests GraphQL endpoint error handling.
func TestUserRead_Error_AddsDiagnostic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal Server Error"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: UserDataSourceSchema(ctx)}}
	(&UserDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error from non-200 HTTP response")
	}
}
