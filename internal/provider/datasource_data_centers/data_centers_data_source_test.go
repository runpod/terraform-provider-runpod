package datasource_data_centers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// Characterization of the systemic GraphQL double-unwrap (CE-1652).
func TestDataCentersRead_DoubleUnwrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"dataCenters":[]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: DataCentersDataSourceSchema(context.Background())}}
	(&DataCentersDataSource{}).Read(context.Background(), datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected double-unwrap failure (CE-1652); if Read now succeeds the bug is fixed — flip to assert the data-center list")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "Failed to get data from response") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Failed to get data from response', got: %v", resp.Diagnostics)
	}
}
