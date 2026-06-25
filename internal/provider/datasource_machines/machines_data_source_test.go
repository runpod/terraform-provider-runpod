package datasource_machines

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// Characterization of the systemic GraphQL double-unwrap (CE-1652): client.Query
// returns the inner data, but Read unwraps result["data"] again (nil) and errors.
// When the bug is fixed this will succeed — flip to assert the machines list.
func TestMachinesRead_DoubleUnwrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"myself":{"machines":[]}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: MachinesDataSourceSchema(context.Background())}}
	(&MachinesDataSource{}).Read(context.Background(), datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected double-unwrap failure (CE-1652); if Read now succeeds the bug is fixed — flip to assert the machines list")
	}
	if !hasDetail(resp, "Failed to get data from response") {
		t.Errorf("expected 'Failed to get data from response', got: %v", resp.Diagnostics)
	}
}

func hasDetail(resp *datasource.ReadResponse, want string) bool {
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), want) {
			return true
		}
	}
	return false
}
