package datasource_billing_network_volume

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestBillingNetworkVolumeDataSourceRead_PopulatesBillingRecords asserts the
// CORRECT behavior of this data source.
//
// The generated schema root (BillingNetworkVolumeDataSourceSchema /
// BillingNetworkVolumeModel in data_source_gen.go) is a single OBJECT with a
// `billing_records` ListNestedAttribute. Given a valid `{"records":[{...}]}` REST
// response, Read should populate the parent model's BillingRecords list and
// produce NO diagnostics errors.
func TestBillingNetworkVolumeDataSourceRead_PopulatesBillingRecords(t *testing.T) {


	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RestQuery requests baseURL + "/billing/networkvolumes" and decodes this JSON
		// object. Read then reads result["billing"] as an array.
		_, _ = w.Write([]byte(`{"records":[{
			"totalAmount":4.56,
			"diskSpaceBilledGb":100,
			"networkVolumeId":"nv-123",
			"startTime":"2026-06-01T00:00:00Z",
			"timeBilledMs":3600000
		}]}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	ctx := context.Background()
	sch := BillingNetworkVolumeDataSourceSchema(ctx)

	cfgState := tfsdk.State{Schema: sch}
	diags := cfgState.Set(ctx, &BillingNetworkVolumeModel{
		BucketSize:      types.StringValue("day"),
		EndTime:         types.StringNull(),
		NetworkVolumeId: types.StringValue("nv-123"),
	})
	if diags.HasError() {
		t.Fatalf("failed to build config state: %v", diags)
	}

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: cfgState.Raw}}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}

	(&BillingNetworkVolumeDataSource{}).Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics errors after a valid billing response; got %v", resp.Diagnostics)
	}

	var got BillingNetworkVolumeModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatalf("failed to read populated state: %v", d)
	}

	if len(got.BillingRecords) != 1 {
		t.Fatalf("expected 1 billing record in state, got %d", len(got.BillingRecords))
	}

	rec := got.BillingRecords[0]
	if rec.Amount.ValueFloat64() != 4.56 {
		t.Errorf("amount: expected 4.56, got %v", rec.Amount.ValueFloat64())
	}
	if rec.DiskSpaceBilledGb.ValueInt64() != 100 {
		t.Errorf("disk_space_billed_gb: expected 100, got %d", rec.DiskSpaceBilledGb.ValueInt64())
	}
	if rec.NetworkVolumeId.ValueString() != "nv-123" {
		t.Errorf("network_volume_id: expected %q, got %q", "nv-123", rec.NetworkVolumeId.ValueString())
	}
	if rec.Time.ValueString() != "2026-06-01T00:00:00Z" {
		t.Errorf("time: expected %q, got %q", "2026-06-01T00:00:00Z", rec.Time.ValueString())
	}
	if rec.TimeBilledMs.ValueInt64() != 3600000 {
		t.Errorf("time_billed_ms: expected 3600000, got %d", rec.TimeBilledMs.ValueInt64())
	}
}
