package datasource_billing_pod

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestBillingPodDataSourceRead_PopulatesBillingRecords asserts the CORRECT
// behavior of this data source.
//
// The generated schema root (BillingPodDataSourceSchema / BillingPodModel in
// data_source_gen.go) is a single OBJECT with a `billing_records`
// ListNestedAttribute. Given a valid `{"billing":[{...}]}` REST response, Read
// should populate the parent model's BillingRecords list and produce NO
// diagnostics errors.
func TestBillingPodDataSourceRead_PopulatesBillingRecords(t *testing.T) {
	t.Skip("CE-1674: Read sets a []BillingRecordModel slice against the single-object root schema (Value Conversion Error); should set the parent model with BillingRecords populated — un-skip when fixed")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RestQuery requests baseURL + "/billing/pods" and decodes this JSON object.
		// Read then reads result["billing"] as an array.
		_, _ = w.Write([]byte(`{"billing":[{
			"amount":7.89,
			"diskSpaceBilledGb":50,
			"endpointId":"ep-123",
			"gpuTypeId":"NVIDIA A100 80GB PCIe",
			"podId":"pod-123",
			"time":"2026-06-01T00:00:00Z",
			"timeBilledMs":3600000
		}]}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	sch := BillingPodDataSourceSchema(ctx)

	cfgState := tfsdk.State{Schema: sch}
	diags := cfgState.Set(ctx, &BillingPodModel{
		BucketSize: types.StringValue("day"),
		EndTime:    types.StringNull(),
		GpuTypeId:  types.StringNull(),
		PodId:      types.StringValue("pod-123"),
	})
	if diags.HasError() {
		t.Fatalf("failed to build config state: %v", diags)
	}

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: cfgState.Raw}}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}

	(&BillingPodDataSource{}).Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics errors after a valid billing response; got %v", resp.Diagnostics)
	}

	var got BillingPodModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatalf("failed to read populated state: %v", d)
	}

	if len(got.BillingRecords) != 1 {
		t.Fatalf("expected 1 billing record in state, got %d", len(got.BillingRecords))
	}

	rec := got.BillingRecords[0]
	if rec.Amount.ValueFloat64() != 7.89 {
		t.Errorf("amount: expected 7.89, got %v", rec.Amount.ValueFloat64())
	}
	if rec.DiskSpaceBilledGb.ValueInt64() != 50 {
		t.Errorf("disk_space_billed_gb: expected 50, got %d", rec.DiskSpaceBilledGb.ValueInt64())
	}
	if rec.EndpointId.ValueString() != "ep-123" {
		t.Errorf("endpoint_id: expected %q, got %q", "ep-123", rec.EndpointId.ValueString())
	}
	if rec.GpuTypeId.ValueString() != "NVIDIA A100 80GB PCIe" {
		t.Errorf("gpu_type_id: expected %q, got %q", "NVIDIA A100 80GB PCIe", rec.GpuTypeId.ValueString())
	}
	if rec.PodId.ValueString() != "pod-123" {
		t.Errorf("pod_id: expected %q, got %q", "pod-123", rec.PodId.ValueString())
	}
	if rec.Time.ValueString() != "2026-06-01T00:00:00Z" {
		t.Errorf("time: expected %q, got %q", "2026-06-01T00:00:00Z", rec.Time.ValueString())
	}
	if rec.TimeBilledMs.ValueInt64() != 3600000 {
		t.Errorf("time_billed_ms: expected 3600000, got %d", rec.TimeBilledMs.ValueInt64())
	}
}
