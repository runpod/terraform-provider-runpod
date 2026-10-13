package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	runpod "github.com/runpod/go-sdk/v2"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

var (
	billingWindowStart = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	billingWindowEnd   = time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
)

func billingQuery() sdktypes.BillingQuery {
	return sdktypes.BillingQuery{
		BucketSize: sdktypes.BillingBucketSizeDay,
		StartTime:  billingWindowStart,
		EndTime:    billingWindowEnd,
	}
}

func TestBillingWindowResolve(t *testing.T) {
	window, diags := billingWindow{
		StartTime:  types.StringValue("2026-06-01T00:00:00Z"),
		EndTime:    types.StringValue("2026-06-02T00:00:00Z"),
		BucketSize: types.StringValue(string(sdktypes.BillingBucketSizeHour)),
		LastN:      types.Int64Value(24),
	}.resolve()
	if diags.HasError() {
		t.Fatal(diags)
	}

	if window.StartTime == nil || !window.StartTime.Equal(billingWindowStart) {
		t.Fatalf("start_time = %v", window.StartTime)
	}
	if window.EndTime == nil || !window.EndTime.Equal(billingWindowEnd) {
		t.Fatalf("end_time = %v", window.EndTime)
	}
	if window.BucketSize == nil || *window.BucketSize != sdktypes.BillingBucketSizeHour {
		t.Fatalf("bucket_size = %v", window.BucketSize)
	}
	if window.LastN == nil || *window.LastN != 24 {
		t.Fatalf("last_n = %v", window.LastN)
	}
}

func TestBillingWindowResolveOmitsUnsetParameters(t *testing.T) {
	window, diags := billingWindow{
		StartTime:  types.StringNull(),
		EndTime:    types.StringNull(),
		BucketSize: types.StringNull(),
		LastN:      types.Int64Null(),
	}.resolve()
	if diags.HasError() {
		t.Fatal(diags)
	}
	if window.StartTime != nil || window.EndTime != nil || window.BucketSize != nil || window.LastN != nil {
		t.Fatalf("unset parameters must stay out of the request: %+v", window)
	}
}

// validateBillingLastN runs the last_n validators of a billing data source
// against a whole configuration, which is what ConflictsWith inspects.
func validateBillingLastN(t *testing.T, newDataSource func() datasource.DataSource, config any) validator.Int64Response {
	t.Helper()
	ctx := context.Background()

	var schemaResp datasource.SchemaResponse
	newDataSource().Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	objectType, ok := schemaResp.Schema.Type().(types.ObjectType)
	if !ok {
		t.Fatalf("schema type = %T, want types.ObjectType", schemaResp.Schema.Type())
	}
	object, diags := types.ObjectValueFrom(ctx, objectType.AttributeTypes(), config)
	if diags.HasError() {
		t.Fatal(diags)
	}
	raw, err := object.ToTerraformValue(ctx)
	if err != nil {
		t.Fatal(err)
	}

	attribute, ok := schemaResp.Schema.Attributes["last_n"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("last_n is %T, want schema.Int64Attribute", schemaResp.Schema.Attributes["last_n"])
	}

	var resp validator.Int64Response
	for _, attributeValidator := range attribute.Int64Validators() {
		attributeValidator.ValidateInt64(ctx, validator.Int64Request{
			Path:           path.Root("last_n"),
			PathExpression: path.MatchRoot("last_n"),
			Config:         tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
			ConfigValue:    types.Int64Value(7),
		}, &resp)
	}
	return resp
}

func TestBillingLastNRejectsAnExplicitWindow(t *testing.T) {
	// The spec has lastN mutually exclusive with startTime/endTime; setting
	// both has to fail at plan time rather than as an API 400.
	resp := validateBillingLastN(t, NewBillingDataSource, billingDataSourceModel{
		LastN:     types.Int64Value(7),
		StartTime: types.StringValue("2026-06-01T00:00:00Z"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("last_n together with start_time must be rejected")
	}

	resp = validateBillingLastN(t, NewPodBillingDataSource, podBillingDataSourceModel{
		LastN:   types.Int64Value(7),
		EndTime: types.StringValue("2026-07-01T00:00:00Z"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("last_n together with end_time must be rejected")
	}
}

func TestBillingLastNAloneIsAccepted(t *testing.T) {
	resp := validateBillingLastN(t, NewBillingDataSource, billingDataSourceModel{
		LastN: types.Int64Value(7),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("last_n on its own must be accepted: %v", resp.Diagnostics)
	}
}

func TestBillingWindowRejectsMalformedTimestamp(t *testing.T) {
	_, diags := billingWindow{StartTime: types.StringValue("2026-06-01"), EndTime: types.StringNull()}.resolve()
	if !diags.HasError() {
		t.Fatal("a non-RFC-3339 start_time must produce an error diagnostic")
	}
}

// TestBillingDataSourcesReadNullConfiguration drives every billing route's
// real Read the way Terraform does. Everything besides the window and the
// route's own filter is computed, so configuration carries metadata as null,
// and decoding the whole state model out of configuration would reject that
// null before any request went out. Each case also pins what
// the configured attributes become on the wire and in state, so a read that
// merely stops erroring is not enough to pass.
func TestBillingDataSourcesReadNullConfiguration(t *testing.T) {
	ctx := context.Background()

	// What a case reads back out of state, one shape for the six models.
	type readState struct {
		bucketSize types.String
		filter     types.String
		total      types.Float64
	}

	bucketSizeDay := map[string]tftypes.Value{"bucket_size": tftypes.NewValue(tftypes.String, "day")}
	filtered := func(name, id string) map[string]tftypes.Value {
		return map[string]tftypes.Value{
			"bucket_size": tftypes.NewValue(tftypes.String, "day"),
			name:          tftypes.NewValue(tftypes.String, id),
		}
	}

	for _, testCase := range []struct {
		name       string
		config     map[string]tftypes.Value
		body       string
		wantPath   string
		wantQuery  string
		wantFilter string
		read       func(t *testing.T, client *runpod.Client, config map[string]tftypes.Value) readState
	}{
		{
			name:   "runpod_billing",
			config: bucketSizeDay,
			body: `{"metadata":{"query":{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"day"},` +
				`"recordCount":0,"totals":{"totalAmount":42.34}},"records":[]}`,
			wantPath:  "/v2/billing",
			wantQuery: "bucketSize=day",
			read: func(t *testing.T, client *runpod.Client, config map[string]tftypes.Value) readState {
				var state billingDataSourceModel
				readDataSource(ctx, t, &billingDataSource{client: client}, config, &state)
				return readState{state.BucketSize, types.StringNull(), state.Metadata.Totals.TotalAmount}
			},
		},
		{
			name:   "runpod_pod_billing",
			config: filtered("pod_id", "pod_abc123"),
			body: `{"metadata":{"query":{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"day","podId":"pod_abc123"},` +
				`"recordCount":0,"uniquePodCount":1,"totals":{"totalAmount":42.34}},"records":[]}`,
			wantPath:   "/v2/billing/pods",
			wantQuery:  "bucketSize=day&podId=pod_abc123",
			wantFilter: "pod_abc123",
			read: func(t *testing.T, client *runpod.Client, config map[string]tftypes.Value) readState {
				var state podBillingDataSourceModel
				readDataSource(ctx, t, &podBillingDataSource{client: client}, config, &state)
				return readState{state.BucketSize, state.PodId, state.Metadata.Totals.TotalAmount}
			},
		},
		{
			name:   "runpod_serverless_billing",
			config: filtered("serverless_id", "ep_abc123"),
			body: `{"metadata":{"query":{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"day","serverlessId":"ep_abc123"},` +
				`"recordCount":0,"uniqueServerlessCount":1,"totals":{"totalAmount":42.34}},"records":[]}`,
			wantPath:   "/v2/billing/serverless",
			wantQuery:  "bucketSize=day&serverlessId=ep_abc123",
			wantFilter: "ep_abc123",
			read: func(t *testing.T, client *runpod.Client, config map[string]tftypes.Value) readState {
				var state serverlessBillingDataSourceModel
				readDataSource(ctx, t, &serverlessBillingDataSource{client: client}, config, &state)
				return readState{state.BucketSize, state.ServerlessId, state.Metadata.Totals.TotalAmount}
			},
		},
		{
			name:   "runpod_endpoint_billing",
			config: bucketSizeDay,
			body: `{"metadata":{"query":{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"day"},` +
				`"recordCount":0,"totals":{"totalAmount":42.34}},"records":[]}`,
			wantPath:  "/v2/billing/endpoints",
			wantQuery: "bucketSize=day",
			read: func(t *testing.T, client *runpod.Client, config map[string]tftypes.Value) readState {
				var state endpointBillingDataSourceModel
				readDataSource(ctx, t, &endpointBillingDataSource{client: client}, config, &state)
				return readState{state.BucketSize, types.StringNull(), state.Metadata.Totals.TotalAmount}
			},
		},
		{
			name:   "runpod_network_volume_billing",
			config: filtered("network_volume_id", "nv_abc123"),
			body: `{"metadata":{"query":{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"day","networkVolumeId":"nv_abc123"},` +
				`"recordCount":0,"uniqueNetworkVolumeCount":1,"totals":{"standardAmount":40,"highPerformanceAmount":2.34,"totalAmount":42.34}},"records":[]}`,
			wantPath:   "/v2/billing/network-volumes",
			wantQuery:  "bucketSize=day&networkVolumeId=nv_abc123",
			wantFilter: "nv_abc123",
			read: func(t *testing.T, client *runpod.Client, config map[string]tftypes.Value) readState {
				var state networkVolumeBillingDataSourceModel
				readDataSource(ctx, t, &networkVolumeBillingDataSource{client: client}, config, &state)
				return readState{state.BucketSize, state.NetworkVolumeId, state.Metadata.Totals.TotalAmount}
			},
		},
		{
			name:   "runpod_cluster_billing",
			config: filtered("cluster_id", "cl_abc123"),
			body: `{"metadata":{"query":{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"day","clusterId":"cl_abc123"},` +
				`"recordCount":0,"uniqueClusterCount":1,"totals":{"gpuAmount":40,"diskAmount":2,"networkingAmount":0.34,"totalAmount":42.34}},"records":[]}`,
			wantPath:   "/v2/billing/clusters",
			wantQuery:  "bucketSize=day&clusterId=cl_abc123",
			wantFilter: "cl_abc123",
			read: func(t *testing.T, client *runpod.Client, config map[string]tftypes.Value) readState {
				var state clusterBillingDataSourceModel
				readDataSource(ctx, t, &clusterBillingDataSource{client: client}, config, &state)
				return readState{state.BucketSize, state.ClusterId, state.Metadata.Totals.TotalAmount}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var gotPath, gotQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer srv.Close()

			state := testCase.read(t, shimTestClient(t, srv.URL), testCase.config)

			if gotPath != testCase.wantPath || gotQuery != testCase.wantQuery {
				t.Errorf("request = %s?%s, want %s?%s", gotPath, gotQuery, testCase.wantPath, testCase.wantQuery)
			}
			if state.bucketSize.ValueString() != "day" {
				t.Errorf("bucket_size in state = %v, want the configured value echoed back", state.bucketSize)
			}
			if state.filter.ValueString() != testCase.wantFilter {
				t.Errorf("filter in state = %v, want %q", state.filter, testCase.wantFilter)
			}
			if state.total.ValueFloat64() != 42.34 {
				t.Errorf("metadata.totals.total_amount = %v", state.total)
			}
		})
	}
}

func TestFlattenBilling(t *testing.T) {
	var state billingDataSourceModel

	flattenBilling(sdktypes.ListBillingResponse{
		Metadata: sdktypes.BillingMetadata{
			Query:       billingQuery(),
			RecordCount: 1,
			Totals: sdktypes.BillingAmounts{
				PodGpuAmount:   12.34,
				EndpointAmount: 3.21,
				TotalAmount:    42.34,
			},
		},
		Records: []sdktypes.BillingRecord{{
			StartTime:                    billingWindowStart,
			EndTime:                      billingWindowEnd,
			PodGpuAmount:                 12.34,
			PodCpuAmount:                 1,
			PodDiskAmount:                2,
			ServerlessGpuAmount:          3,
			ServerlessCpuAmount:          4,
			ServerlessDiskAmount:         5,
			ClusterGpuAmount:             7,
			ClusterDiskAmount:            8,
			ClusterNetworkingAmount:      9,
			StorageStandardAmount:        10,
			StorageHighPerformanceAmount: 11,
			EndpointAmount:               3.21,
			TotalAmount:                  42.34,
		}},
	}, &state)

	if state.Metadata.RecordCount.ValueInt64() != 1 {
		t.Fatalf("record_count = %v", state.Metadata.RecordCount)
	}
	if state.Metadata.Query.StartTime.ValueString() != "2026-06-01T00:00:00Z" ||
		state.Metadata.Query.EndTime.ValueString() != "2026-06-02T00:00:00Z" ||
		state.Metadata.Query.BucketSize.ValueString() != "day" {
		t.Fatalf("query: %+v", state.Metadata.Query)
	}
	if state.Metadata.Totals.TotalAmount.ValueFloat64() != 42.34 ||
		state.Metadata.Totals.PodGpuAmount.ValueFloat64() != 12.34 {
		t.Fatalf("totals: %+v", state.Metadata.Totals)
	}
	if len(state.Records) != 1 {
		t.Fatalf("records = %d, want 1", len(state.Records))
	}
	record := state.Records[0]
	if record.StartTime.ValueString() != "2026-06-01T00:00:00Z" {
		t.Fatalf("record start_time = %q", record.StartTime.ValueString())
	}
	if record.Amounts.TotalAmount.ValueFloat64() != 42.34 ||
		record.Amounts.EndpointAmount.ValueFloat64() != 3.21 ||
		record.Amounts.StorageHighPerformanceAmount.ValueFloat64() != 11 {
		t.Fatalf("record amounts: %+v", record.Amounts)
	}

	assertDataSourceModelFits(t, NewBillingDataSource, state)
}

func TestFlattenPodBilling(t *testing.T) {
	podId := "pod_abc123"
	var state podBillingDataSourceModel

	flattenPodBilling(sdktypes.ListPodBillingResponse{
		Metadata: sdktypes.PodBillingMetadata{
			Query: sdktypes.PodBillingQuery{
				BucketSize: sdktypes.BillingBucketSizeDay,
				StartTime:  billingWindowStart,
				EndTime:    billingWindowEnd,
				PodId:      &podId,
			},
			RecordCount:    1,
			UniquePodCount: 1,
			Totals: sdktypes.PodBillingAmounts{
				GpuAmount:   10,
				CpuAmount:   1,
				DiskAmount:  1.34,
				TotalAmount: 12.34,
			},
		},
		Records: []sdktypes.PodBillingRecord{{
			PodId:       podId,
			StartTime:   billingWindowStart,
			EndTime:     billingWindowEnd,
			GpuAmount:   10,
			CpuAmount:   1,
			DiskAmount:  1.34,
			TotalAmount: 12.34,
		}},
	}, &state)

	if state.Metadata.Query.PodId.ValueString() != podId ||
		state.Metadata.UniquePodCount.ValueInt64() != 1 {
		t.Fatalf("metadata: %+v", state.Metadata)
	}
	if state.Metadata.Totals.TotalAmount.ValueFloat64() != 12.34 {
		t.Fatalf("totals: %+v", state.Metadata.Totals)
	}
	if len(state.Records) != 1 || state.Records[0].PodId.ValueString() != podId ||
		state.Records[0].Amounts.GpuAmount.ValueFloat64() != 10 {
		t.Fatalf("records: %+v", state.Records)
	}

	assertDataSourceModelFits(t, NewPodBillingDataSource, state)
}

func TestFlattenServerlessBilling(t *testing.T) {
	var state serverlessBillingDataSourceModel

	flattenServerlessBilling(sdktypes.ListServerlessBillingResponse{
		Metadata: sdktypes.ServerlessBillingMetadata{
			Query: sdktypes.ServerlessBillingQuery{
				BucketSize: sdktypes.BillingBucketSizeDay,
				StartTime:  billingWindowStart,
				EndTime:    billingWindowEnd,
			},
			RecordCount:           2,
			UniqueServerlessCount: 2,
			Totals: sdktypes.ServerlessBillingAmounts{
				GpuAmount:   6,
				CpuAmount:   1,
				DiskAmount:  1,
				TotalAmount: 8,
			},
		},
		Records: []sdktypes.ServerlessBillingRecord{
			{ServerlessId: "ep_one", StartTime: billingWindowStart, EndTime: billingWindowEnd, GpuAmount: 4, TotalAmount: 4},
			{ServerlessId: "ep_two", StartTime: billingWindowStart, EndTime: billingWindowEnd, GpuAmount: 2, CpuAmount: 1, DiskAmount: 1, TotalAmount: 4},
		},
	}, &state)

	if !state.Metadata.Query.ServerlessId.IsNull() {
		t.Fatalf("serverless_id echo = %v, want null when unfiltered", state.Metadata.Query.ServerlessId)
	}
	if state.Metadata.UniqueServerlessCount.ValueInt64() != 2 ||
		state.Metadata.Totals.TotalAmount.ValueFloat64() != 8 {
		t.Fatalf("metadata: %+v", state.Metadata)
	}
	if len(state.Records) != 2 || state.Records[1].ServerlessId.ValueString() != "ep_two" ||
		state.Records[1].Amounts.DiskAmount.ValueFloat64() != 1 {
		t.Fatalf("records: %+v", state.Records)
	}

	assertDataSourceModelFits(t, NewServerlessBillingDataSource, state)
}

func TestFlattenEndpointBilling(t *testing.T) {
	var state endpointBillingDataSourceModel

	flattenEndpointBilling(sdktypes.ListEndpointBillingResponse{
		Metadata: sdktypes.EndpointBillingMetadata{
			Query:       billingQuery(),
			RecordCount: 1,
			Totals:      sdktypes.EndpointBillingAmounts{TotalAmount: 3.21},
		},
		Records: []sdktypes.EndpointBillingRecord{{
			StartTime:   billingWindowStart,
			EndTime:     billingWindowEnd,
			TotalAmount: 3.21,
		}},
	}, &state)

	if state.Metadata.Totals.TotalAmount.ValueFloat64() != 3.21 {
		t.Fatalf("totals: %+v", state.Metadata.Totals)
	}
	if len(state.Records) != 1 || state.Records[0].Amounts.TotalAmount.ValueFloat64() != 3.21 {
		t.Fatalf("records: %+v", state.Records)
	}

	assertDataSourceModelFits(t, NewEndpointBillingDataSource, state)
}

func TestFlattenNetworkVolumeBilling(t *testing.T) {
	volumeId := "vol_abc123"
	var state networkVolumeBillingDataSourceModel

	flattenNetworkVolumeBilling(sdktypes.ListNetworkVolumeBillingResponse{
		Metadata: sdktypes.NetworkVolumeBillingMetadata{
			Query: sdktypes.NetworkVolumeBillingQuery{
				BucketSize:      sdktypes.BillingBucketSizeMonth,
				StartTime:       billingWindowStart,
				EndTime:         billingWindowEnd,
				NetworkVolumeId: &volumeId,
			},
			RecordCount:              1,
			UniqueNetworkVolumeCount: 1,
			Totals: sdktypes.NetworkVolumeBillingAmounts{
				StandardAmount:        4.79,
				HighPerformanceAmount: 1,
				TotalAmount:           5.79,
			},
		},
		Records: []sdktypes.NetworkVolumeBillingRecord{{
			NetworkVolumeId:       volumeId,
			StartTime:             billingWindowStart,
			EndTime:               billingWindowEnd,
			StandardAmount:        4.79,
			HighPerformanceAmount: 1,
			TotalAmount:           5.79,
		}},
	}, &state)

	if state.Metadata.Query.BucketSize.ValueString() != "month" ||
		state.Metadata.Query.NetworkVolumeId.ValueString() != volumeId {
		t.Fatalf("query: %+v", state.Metadata.Query)
	}
	if state.Metadata.UniqueNetworkVolumeCount.ValueInt64() != 1 {
		t.Fatalf("unique count: %+v", state.Metadata)
	}
	if len(state.Records) != 1 || state.Records[0].Amounts.StandardAmount.ValueFloat64() != 4.79 ||
		state.Records[0].Amounts.HighPerformanceAmount.ValueFloat64() != 1 {
		t.Fatalf("records: %+v", state.Records)
	}

	assertDataSourceModelFits(t, NewNetworkVolumeBillingDataSource, state)
}

func TestFlattenClusterBilling(t *testing.T) {
	var state clusterBillingDataSourceModel

	flattenClusterBilling(sdktypes.ListClusterBillingResponse{
		Metadata: sdktypes.ClusterBillingMetadata{
			Query: sdktypes.ClusterBillingQuery{
				BucketSize: sdktypes.BillingBucketSizeDay,
				StartTime:  billingWindowStart,
				EndTime:    billingWindowEnd,
			},
			RecordCount:        1,
			UniqueClusterCount: 1,
			Totals: sdktypes.ClusterBillingAmounts{
				GpuAmount:        90,
				DiskAmount:       5,
				NetworkingAmount: 4,
				TotalAmount:      99,
			},
		},
		Records: []sdktypes.ClusterBillingRecord{{
			ClusterId:        "cluster_abc123",
			StartTime:        billingWindowStart,
			EndTime:          billingWindowEnd,
			GpuAmount:        90,
			DiskAmount:       5,
			NetworkingAmount: 4,
			TotalAmount:      99,
		}},
	}, &state)

	if !state.Metadata.Query.ClusterId.IsNull() {
		t.Fatalf("cluster_id echo = %v, want null when unfiltered", state.Metadata.Query.ClusterId)
	}
	if state.Metadata.Totals.NetworkingAmount.ValueFloat64() != 4 {
		t.Fatalf("totals: %+v", state.Metadata.Totals)
	}
	if len(state.Records) != 1 || state.Records[0].ClusterId.ValueString() != "cluster_abc123" ||
		state.Records[0].Amounts.TotalAmount.ValueFloat64() != 99 {
		t.Fatalf("records: %+v", state.Records)
	}

	assertDataSourceModelFits(t, NewClusterBillingDataSource, state)
}
