package datasource_pod

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

// Characterization of the systemic GraphQL double-unwrap (CE-1652) in the pod
// data source. Read first decodes a config input, so we supply a valid config;
// the failure is the result["data"] re-unwrap, not a config error.
func TestPodDataSourceRead_DoubleUnwrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"pod":{"id":"p1","name":"n"}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := PodDataSourceSchema(ctx)
	cfgState := tfsdk.State{Schema: sch}
	if d := cfgState.Set(ctx, &PodModel{Id: types.StringValue("p1"), Env: types.ListNull(types.StringType)}); d.HasError() {
		t.Fatalf("building config: %v", d)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&PodDataSource{}).Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: cfgState.Raw}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected double-unwrap failure (CE-1652); if Read now succeeds the bug is fixed — flip to assert pod fields")
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
