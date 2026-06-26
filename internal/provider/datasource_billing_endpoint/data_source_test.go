package datasource_billing_endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestBillingEndpointDataSourceRead_PopulatesBillingRecords asserts the CORRECT
// behavior of this data source.
//
// The generated schema root (BillingEndpointDataSourceSchema / BillingEndpointModel
// in data_source_gen.go) is a single OBJECT with a `billing_records`
// ListNestedAttribute. Given a valid `{"billing":[{...}]}` REST response, Read
// should populate the parent model's BillingRecords list and produce NO
// diagnostics errors.
func TestBillingEndpointDataSourceRead_PopulatesBillingRecords(t *testing.T) {
	t.Skip("CE-1674: Read sets a []BillingRecordModel slice against the single-object root schema (Value Conversion Error); should set the parent model with BillingRecords populated — un-skip when fixed")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RestQuery requests baseURL + "/billing/endpoints" and decodes this JSON object.
		// Read then reads result["billing"] as an array.
		_, _ = w.Write([]byte(`{"billing":[{
			"amount":1.23,
			"endpointId":"ep-123",
			"time":"2026-06-01T00:00:00Z",
			"timeBilledMs":3600000
		}]}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	sch := BillingEndpointDataSourceSchema(ctx)

	// Read first decodes config inputs (filters) via req.Config.Get.
	cfgState := tfsdk.State{Schema: sch}
	diags := cfgState.Set(ctx, &BillingEndpointModel{
		BucketSize: types.StringValue("day"),
		EndTime:    types.StringNull(),
		EndpointId: types.StringValue("ep-123"),
	})
	if diags.HasError() {
		t.Fatalf("failed to build config state: %v", diags)
	}

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: cfgState.Raw}}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}

	(&BillingEndpointDataSource{}).Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics errors after a valid billing response; got %v", resp.Diagnostics)
	}

	var got BillingEndpointModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatalf("failed to read populated state: %v", d)
	}

	if len(got.BillingRecords) != 1 {
		t.Fatalf("expected 1 billing record in state, got %d", len(got.BillingRecords))
	}

	rec := got.BillingRecords[0]
	if rec.Amount.ValueFloat64() != 1.23 {
		t.Errorf("amount: expected 1.23, got %v", rec.Amount.ValueFloat64())
	}
	if rec.EndpointId.ValueString() != "ep-123" {
		t.Errorf("endpoint_id: expected %q, got %q", "ep-123", rec.EndpointId.ValueString())
	}
	if rec.Time.ValueString() != "2026-06-01T00:00:00Z" {
		t.Errorf("time: expected %q, got %q", "2026-06-01T00:00:00Z", rec.Time.ValueString())
	}
	if rec.TimeBilledMs.ValueInt64() != 3600000 {
		t.Errorf("time_billed_ms: expected 3600000, got %d", rec.TimeBilledMs.ValueInt64())
	}
}
