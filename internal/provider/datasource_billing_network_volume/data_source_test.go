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

// TestBillingNetworkVolumeDataSourceRead_SliceObjectMismatch characterizes the real
// current behavior of this NEW data source.
//
// The generated schema root (BillingNetworkVolumeDataSourceSchema /
// BillingNetworkVolumeModel in data_source_gen.go) is a single OBJECT with a
// `billing_records` ListNestedAttribute. But Read (data_source.go:92) does
//
//	resp.State.Set(ctx, &models)
//
// where `models` is a []BillingRecordModel SLICE. Setting a slice value against an
// object-root schema produces a terraform-plugin-framework "Value Conversion Error",
// so a perfectly valid `{"billing":[...]}` REST response is never surfaced into state.
func TestBillingNetworkVolumeDataSourceRead_SliceObjectMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RestQuery requests baseURL + "/billing/networkvolumes" and decodes this JSON
		// object. Read then reads result["billing"] as an array. Every field
		// BillingRecordModel reads is present and well-typed.
		_, _ = w.Write([]byte(`{"billing":[{
			"amount":4.56,
			"diskSpaceBilledGb":100,
			"networkVolumeId":"nv-123",
			"time":"2026-06-01T00:00:00Z",
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

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected a slice/object Value Conversion Error from State.Set, got none; diags=%v", resp.Diagnostics)
	}

	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "Value Conversion Error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a %q diagnostic (slice set against object-root schema); got diags=%v",
			"Value Conversion Error", resp.Diagnostics)
	}
}
