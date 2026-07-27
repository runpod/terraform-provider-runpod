package datasource_machine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// newMachineConfig builds a valid Config (Id only) the way TestMachineData-
// SourceRead_PopulatesState does, so Read's config decode succeeds and we reach
// the GraphQL-result branches under test.
func newMachineConfig(ctx context.Context, t *testing.T) tfsdk.Config {
	t.Helper()
	s := MachineDataSourceSchema(ctx)
	cfgState := tfsdk.State{Schema: s}
	if d := cfgState.Set(ctx, &MachineModel{Id: types.StringValue("m1")}); d.HasError() {
		t.Fatalf("building config: %v", d)
	}
	return tfsdk.Config{Schema: s, Raw: cfgState.Raw}
}

// runRead spins up a stub GraphQL server returning body, points the data source
// at it, and runs Read with a valid (Id=m1) config. Returns the response.
func runRead(t *testing.T, body string) *datasource.ReadResponse {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := MachineDataSourceSchema(ctx)
	cfg := newMachineConfig(ctx, t)
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&MachineDataSource{}).Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	return resp
}

// TestMachineDataSourceRead_GraphQLError covers the API-error branch: when the
// GraphQL response carries an "errors" array, rlClient.Query returns an error and
// Read must surface it as a diagnostic (and must NOT set state).
func TestMachineDataSourceRead_GraphQLError(t *testing.T) {
	resp := runRead(t, `{"errors":[{"message":"machine boom"}]}`)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected Read to report a diagnostic error on GraphQL errors, got none")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "API Error" && strings.Contains(d.Detail(), "machine boom") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an \"API Error\" diagnostic mentioning the GraphQL error, got: %v", resp.Diagnostics)
	}
}

// TestMachineDataSourceRead_HTTPError covers the API-error branch via a non-200
// HTTP status (rlClient.Query turns it into an error before any parsing).
func TestMachineDataSourceRead_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server exploded"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := MachineDataSourceSchema(ctx)
	cfg := newMachineConfig(ctx, t)
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&MachineDataSource{}).Read(ctx, datasource.ReadRequest{Config: cfg}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected Read to report a diagnostic error on HTTP 500, got none")
	}
}

// TestMachineDataSourceRead_MachineNotInResponse covers the else branch: the
// envelope is valid and unwrapped to a data object, but it contains no
// "machine" key, so Read must emit the "Machine not found in response"
// diagnostic rather than panicking.
func TestMachineDataSourceRead_MachineNotInResponse(t *testing.T) {
	resp := runRead(t, `{"data":{"somethingElse":{}}}`)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected Read to report a diagnostic error when machine is absent, got none")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "API Error" && d.Detail() == "Machine not found in response" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected \"Machine not found in response\" diagnostic, got: %v", resp.Diagnostics)
	}
}

// TestMachineDataSourceRead_MachineNull covers the same else branch when the
// machine key is present but JSON null (type assertion to map fails).
func TestMachineDataSourceRead_MachineNull(t *testing.T) {
	resp := runRead(t, `{"data":{"machine":null}}`)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected Read to report a diagnostic error when machine is null, got none")
	}
}

// TestNewMachineDataSource smoke-checks the constructor returns a usable,
// non-nil data source of the expected concrete type.
func TestNewMachineDataSource(t *testing.T) {
	ds := NewMachineDataSource()
	if ds == nil {
		t.Fatal("NewMachineDataSource returned nil")
	}
	if _, ok := ds.(*MachineDataSource); !ok {
		t.Fatalf("expected *MachineDataSource, got %T", ds)
	}
}

// TestMachineDataSourceMetadata asserts the type name wiring.
func TestMachineDataSourceMetadata(t *testing.T) {
	resp := &datasource.MetadataResponse{}
	(&MachineDataSource{}).Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "runpod_machine" {
		t.Errorf("expected TypeName %q, got %q", "runpod_machine", resp.TypeName)
	}
}

// TestMachineDataSourceSchema smoke-checks Schema populates the response with
// the expected attributes and no diagnostics.
func TestMachineDataSourceSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	(&MachineDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema reported diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["id"]; !ok {
		t.Errorf("expected schema to contain an \"id\" attribute, attributes=%v", resp.Schema.Attributes)
	}
	if _, ok := resp.Schema.Attributes["host_price_per_gpu"]; !ok {
		t.Errorf("expected schema to contain a \"host_price_per_gpu\" attribute")
	}
}
