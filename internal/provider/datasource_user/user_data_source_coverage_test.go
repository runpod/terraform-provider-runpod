package datasource_user

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// readWithServer wires the data source to a local REST stub and runs Read.
// The stub body is written as-is; rlClient.RestQuery handles v2 response envelope.
func readWithServer(t *testing.T, body string) *datasource.ReadResponse {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: UserDataSourceSchema(ctx)}}
	(&UserDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)
	return resp
}

// TestNewUserDataSource_ReturnsNonNil covers the constructor and confirms it
// returns a usable *UserDataSource implementing the datasource.DataSource interface.
func TestNewUserDataSource_ReturnsNonNil(t *testing.T) {
	ds := NewUserDataSource()
	if ds == nil {
		t.Fatal("NewUserDataSource returned nil")
	}
	if _, ok := ds.(*UserDataSource); !ok {
		t.Fatalf("expected *UserDataSource, got %T", ds)
	}
}

// TestMetadata_SetsTypeName covers Metadata and asserts the fixed type name.
func TestMetadata_SetsTypeName(t *testing.T) {
	resp := &datasource.MetadataResponse{}
	(&UserDataSource{}).Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "runpod_user" {
		t.Errorf("expected TypeName %q, got %q", "runpod_user", resp.TypeName)
	}
}

// TestSchema_PopulatesAttributes covers Schema and confirms the user
// attributes (id, pubKey) are present on the returned schema.
func TestSchema_PopulatesAttributes(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	(&UserDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("Schema returned no attributes")
	}
	for _, name := range []string{"id", "pub_key"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("expected attribute %q in schema, attributes=%v", name, resp.Schema.Attributes)
		}
	}
}

// TestRead_RestError_AddsDiagnostic covers the error branch where the
// server returns a REST API error response.
func TestRead_RestError_AddsDiagnostic(t *testing.T) {
	resp := readWithServer(t, `{"error":"Unauthorized"}`)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error from REST error response")
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

// TestRead_HTTPError_AddsDiagnostic covers the same error branch reached via a
// non-200 HTTP status from the REST endpoint.
func TestRead_HTTPError_AddsDiagnostic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: UserDataSourceSchema(ctx)}}
	(&UserDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error from non-200 HTTP response")
	}
}

// TestRead_DataMissingInResponse_AddsDiagnostic covers the else branch: the
// response decodes but has no `data` object, so Read reports
// "User data not found in response".
func TestRead_DataMissingInResponse_AddsDiagnostic(t *testing.T) {
	resp := readWithServer(t, `{"somethingElse":{}}`)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error when data is missing from response")
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
