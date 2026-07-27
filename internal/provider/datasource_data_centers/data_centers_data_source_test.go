package datasource_data_centers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// CE-1652 (GraphQL double-unwrap) is FIXED by PR #20: client.Query() returns the
// inner "data" map and Read() reads result["dataCenter"] directly. This data
// source is still non-functional because of a SEPARATE, pre-existing bug that
// #20 did not touch: Read builds a []DataCentersModel slice and calls State.Set
// against the single-object root schema in data_centers_data_source_gen.go,
// which the framework rejects ("must be an attr.TypeWithElementType ... Value
// Conversion Error").
//
// This characterizes the current state: the old double-unwrap error is gone
// (proving the #20 fix reached this data source), but Read still errors in
// State.Set. When the schema/Read shape is fixed (schema becomes a list/nested
// attribute, or Read sets a single object), State.Set will succeed — flip this
// to assert the parsed data-center list.
func TestDataCentersRead_PopulatesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/catalog/datacenters" {
			_, _ = w.Write([]byte(`{"dataCenters":[{"id":"US-CA-1","name":"California 1","location":"US","globalNetwork":true}]}`))
		}
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: DataCentersDataSourceSchema(ctx)}}
	(&DataCentersDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to succeed, got diags=%v", resp.Diagnostics)
	}

	var state DataCentersDataSourceModel
	diags := resp.State.Get(ctx, &state)
	if diags.HasError() {
		t.Fatalf("expected to read state back, got diags=%v", diags)
	}
	if len(state.DataCenters) != 1 {
		t.Fatalf("expected 1 data center, got %d", len(state.DataCenters))
	}
	if state.DataCenters[0].Id != types.StringValue("US-CA-1") {
		t.Errorf("data center ID: want %q, got %v", "US-CA-1", state.DataCenters[0].Id)
	}
	if state.DataCenters[0].Name != types.StringValue("California 1") {
		t.Errorf("data center name: want %q, got %v", "California 1", state.DataCenters[0].Name)
	}
}
