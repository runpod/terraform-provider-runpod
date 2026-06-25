package datasource_gpu_types

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// TestGpuTypesRead_DoubleUnwrap_R1 shows that R1 (the result["data"] double-
// unwrap) is not limited to the resources — the GraphQL data sources have it
// too. client.Query() already returns the inner data, so result["data"] is nil
// and Read errors with "Failed to get data from response" even on a valid
// response. Until R1 is fixed, this data source always fails.
func TestGpuTypesRead_DoubleUnwrap_R1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"gpus":[{"id":"g1","displayName":"A100","manufacturer":"NVIDIA","cuda_cores":6912,"memory_in_gb":80,"community_price":1.0,"secure_price":2.0,"secure_cloud":true}]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	sch := GpuTypesDataSourceSchema(context.Background())
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&GpuTypesDataSource{}).Read(context.Background(), datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected R1 double-unwrap failure; if Read now succeeds, R1 is FIXED — flip to assert the gpu list")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "Failed to get data from response") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Failed to get data from response' (R1), got: %v", resp.Diagnostics)
	}
}
