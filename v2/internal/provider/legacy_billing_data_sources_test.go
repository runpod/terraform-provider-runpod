package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

func billingFloatPtr(value float64) *float64 { return &value }
func billingStringPtr(value string) *string  { return &value }

func TestFlattenLegacyPodBilling(t *testing.T) {
	records := flattenLegacyPodBilling([]legacy.PodBillingRecordV1{{
		Amount:            billingFloatPtr(1.25),
		DiskSpaceBilledGb: billingFloatPtr(40),
		EndpointID:        billingStringPtr("endpoint_abc"),
		GPUTypeID:         billingStringPtr("NVIDIA A100 80GB PCIe"),
		PodID:             billingStringPtr("pod_abc"),
		Time:              billingStringPtr("2026-06-01T00:00:00Z"),
		TimeBilledMs:      billingFloatPtr(3600000),
	}})

	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Amount.ValueFloat64() != 1.25 || record.Time.ValueString() != "2026-06-01T00:00:00Z" {
		t.Errorf("amount/time: %+v", record)
	}
	if record.DiskSpaceBilledGb.ValueInt64() != 40 || record.TimeBilledMs.ValueInt64() != 3600000 {
		t.Errorf("billed quantities: %+v", record)
	}
	if record.EndpointId.ValueString() != "endpoint_abc" || record.PodId.ValueString() != "pod_abc" {
		t.Errorf("ids: %+v", record)
	}
	if record.GpuTypeId.ValueString() != "NVIDIA A100 80GB PCIe" {
		t.Errorf("gpu_type_id = %v", record.GpuTypeId)
	}
}

func TestFlattenLegacyPodBillingNullsAbsentFields(t *testing.T) {
	// The REST v1 route omits the optional per-record fields; 1.0.8 read amount
	// and time with unchecked assertions that panicked when either was absent.
	records := flattenLegacyPodBilling([]legacy.PodBillingRecordV1{{}})

	record := records[0]
	if !record.Amount.IsNull() || !record.Time.IsNull() {
		t.Errorf("amount/time should read null: %+v", record)
	}
	if !record.DiskSpaceBilledGb.IsNull() || !record.TimeBilledMs.IsNull() ||
		!record.EndpointId.IsNull() || !record.GpuTypeId.IsNull() || !record.PodId.IsNull() {
		t.Errorf("absent fields should read null: %+v", record)
	}
}

func TestFlattenLegacyPodBillingTruncatesBilledQuantities(t *testing.T) {
	// Both are Int64 attributes in the 1.x schema and the route answers JSON
	// numbers, so a fractional value truncates the way 1.0.8 truncated it.
	records := flattenLegacyPodBilling([]legacy.PodBillingRecordV1{{
		DiskSpaceBilledGb: billingFloatPtr(39.75),
		TimeBilledMs:      billingFloatPtr(1500.9),
	}})

	if records[0].DiskSpaceBilledGb.ValueInt64() != 39 || records[0].TimeBilledMs.ValueInt64() != 1500 {
		t.Errorf("truncation: %+v", records[0])
	}
}

func TestFlattenLegacyNetworkVolumeBilling(t *testing.T) {
	records := flattenLegacyNetworkVolumeBilling([]legacy.NetworkVolumeBillingRecordV1{
		{
			Amount:            billingFloatPtr(0.5),
			DiskSpaceBilledGb: billingFloatPtr(100),
			NetworkVolumeID:   billingStringPtr("netvol_abc"),
			Time:              billingStringPtr("2026-06-01T00:00:00Z"),
			TimeBilledMs:      billingFloatPtr(86400000),
		},
		{},
	})

	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].Amount.ValueFloat64() != 0.5 || records[0].DiskSpaceBilledGb.ValueInt64() != 100 ||
		records[0].NetworkVolumeId.ValueString() != "netvol_abc" || records[0].TimeBilledMs.ValueInt64() != 86400000 {
		t.Errorf("first record: %+v", records[0])
	}
	if !records[1].Amount.IsNull() || !records[1].NetworkVolumeId.IsNull() || !records[1].TimeBilledMs.IsNull() {
		t.Errorf("absent fields should read null: %+v", records[1])
	}
}

func TestFlattenLegacyEndpointBilling(t *testing.T) {
	records := flattenLegacyEndpointBilling([]legacy.EndpointBillingRecordV1{
		{
			Amount:       billingFloatPtr(2),
			EndpointID:   billingStringPtr("endpoint_abc"),
			Time:         billingStringPtr("2026-06-01T00:00:00Z"),
			TimeBilledMs: billingFloatPtr(120000),
		},
		{},
	})

	if records[0].Amount.ValueFloat64() != 2 || records[0].EndpointId.ValueString() != "endpoint_abc" ||
		records[0].TimeBilledMs.ValueInt64() != 120000 {
		t.Errorf("first record: %+v", records[0])
	}
	if !records[1].EndpointId.IsNull() || !records[1].TimeBilledMs.IsNull() {
		t.Errorf("absent fields should read null: %+v", records[1])
	}
}

func TestFlattenLegacyBillingEmptyResponse(t *testing.T) {
	// An account with no spend answers with an empty array, which is an empty
	// list in state rather than null.
	if records := flattenLegacyPodBilling(nil); records == nil || len(records) != 0 {
		t.Errorf("pod records = %+v, want an empty list", records)
	}
	if records := flattenLegacyNetworkVolumeBilling(nil); records == nil || len(records) != 0 {
		t.Errorf("network volume records = %+v, want an empty list", records)
	}
	if records := flattenLegacyEndpointBilling(nil); records == nil || len(records) != 0 {
		t.Errorf("endpoint records = %+v, want an empty list", records)
	}
}

func TestLegacyBillingQueriesCarryTheConfiguredFilters(t *testing.T) {
	pod := legacyPodBillingModel{
		BucketSize: types.StringValue("hour"),
		EndTime:    types.StringValue("2026-06-02T00:00:00Z"),
		GpuTypeId:  types.StringValue("NVIDIA A100 80GB PCIe"),
		PodId:      types.StringValue("pod_abc"),
	}.query()
	if pod.BucketSize != "hour" || pod.EndTime != "2026-06-02T00:00:00Z" ||
		pod.GPUTypeID != "NVIDIA A100 80GB PCIe" || pod.PodID != "pod_abc" {
		t.Errorf("pod query: %+v", pod)
	}

	volume := legacyNetworkVolumeBillingModel{
		BucketSize:      types.StringValue("day"),
		NetworkVolumeId: types.StringValue("netvol_abc"),
	}.query()
	if volume.BucketSize != "day" || volume.NetworkVolumeID != "netvol_abc" || volume.EndTime != "" {
		t.Errorf("network volume query: %+v", volume)
	}

	endpoint := legacyEndpointBillingModel{EndpointId: types.StringValue("endpoint_abc")}.query()
	if endpoint.EndpointID != "endpoint_abc" || endpoint.BucketSize != "" {
		t.Errorf("endpoint query: %+v", endpoint)
	}
}

func TestAppendLegacyBillingFailureAlwaysErrors(t *testing.T) {
	// These three data sources have no source but the legacy host: the records
	// are the whole body. Answering a failed read with null records hands the
	// configuration "Attempt to index null value" and leaves the cause in a
	// warning, so a failure is an error whether or not a filter is set. A 404
	// is a failure here too, unlike the shim read-backs.
	var unfiltered diag.Diagnostics
	appendLegacyBillingFailure(&unfiltered, "pod billing", errors.New("host is gone"))
	if !unfiltered.HasError() {
		t.Fatalf("an unfiltered read must error: %v", unfiltered)
	}
	if !strings.Contains(unfiltered.Errors()[0].Detail(), "host is gone") {
		t.Errorf("the error must carry the cause: %q", unfiltered.Errors()[0].Detail())
	}

	var notFound diag.Diagnostics
	appendLegacyBillingFailure(&notFound, "pod billing", &legacy.NotFoundError{Path: "/billing/pods"})
	if !notFound.HasError() {
		t.Fatalf("a 404 must error rather than answer with null records: %v", notFound)
	}
}

func TestLegacyPodBillingErrorsWhenTheRouteFails(t *testing.T) {
	// The unfiltered case: data "runpod_billing_pod" "all" {} against a v1
	// host answering 500. The read must fail rather than write
	// billing_records = null and warn.
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"billing is down"}`))
	}))
	defer srv.Close()

	// The plain test client: the billing read is a GET, and the shared retry
	// transport would spend seconds backing off before reporting the 500.
	d := &legacyPodBillingDataSource{client: newTestLegacyClient(t, legacy.Config{
		APIKey: "test-key", RestV1URL: srv.URL, HTTPClient: srv.Client(),
	})}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type = %T, want an object", schemaResp.Schema.Type().TerraformType(ctx))
	}
	values := map[string]tftypes.Value{}
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, nil)}}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, values)},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a failed billing read must error, not warn: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a failed read must not write a state whose billing_records are null")
	}
}

func TestLegacyPodBillingReadsTheRestV1Route(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[{"amount":1.25,"time":"2026-06-01T00:00:00Z","gpuTypeId":"NVIDIA A100 80GB PCIe","diskSpaceBilledGb":40,"endpointId":"endpoint_abc","podId":"pod_abc","timeBilledMs":3600000}]`))
	}))
	defer srv.Close()

	client := newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL})
	state := legacyPodBillingModel{
		BucketSize: types.StringValue("day"),
		GpuTypeId:  types.StringValue("NVIDIA A100 80GB PCIe"),
	}
	records, err := client.ListPodBillingV1(context.Background(), state.query())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/billing/pods" {
		t.Errorf("path = %q, want /billing/pods", gotPath)
	}
	if gotQuery != "bucketSize=day&gpuTypeId=NVIDIA+A100+80GB+PCIe" {
		t.Errorf("query = %q, want the bucket size and the gpu type filter", gotQuery)
	}

	flattened := flattenLegacyPodBilling(records)
	if len(flattened) != 1 || flattened[0].GpuTypeId.ValueString() != "NVIDIA A100 80GB PCIe" ||
		flattened[0].DiskSpaceBilledGb.ValueInt64() != 40 {
		t.Errorf("flattened: %+v", flattened)
	}
}

func TestLegacyPodBillingErrorsWithoutALegacyClient(t *testing.T) {
	// These data sources read the REST v1 host and nothing else, so a provider
	// that handed over no legacy client has to say so: returning quietly
	// answers the read with "Provider produced a null object", which tells the
	// user nothing.
	ctx := context.Background()
	d := &legacyPodBillingDataSource{}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type = %T, want an object", schemaResp.Schema.Type().TerraformType(ctx))
	}
	values := map[string]tftypes.Value{}
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, nil)}}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, values)},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a read with no legacy client must report why: %v", resp.Diagnostics)
	}
}
