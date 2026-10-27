package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	runpod "github.com/runpod/go-sdk/v2"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

type legacyBillingRequest struct {
	method, path, rawQuery, body string
}

// legacyBillingRecorder is a stub REST API that answers every request with
// one status and body and records what reached it.
type legacyBillingRecorder struct {
	mu       sync.Mutex
	requests []legacyBillingRequest
}

func (r *legacyBillingRecorder) all() []legacyBillingRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]legacyBillingRequest(nil), r.requests...)
}

// legacyBillingStub serves status and body for every request. The client has
// no retry transport, so a 5xx is reported at once.
func legacyBillingStub(t *testing.T, status int, body string) (*runpod.Client, *legacyBillingRecorder) {
	t.Helper()
	recorder := &legacyBillingRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		recorder.mu.Lock()
		recorder.requests = append(recorder.requests, legacyBillingRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(payload)})
		recorder.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	client, err := runpod.New(runpod.WithAPIKey("test-key"), runpod.WithBaseURL(srv.URL), runpod.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return client, recorder
}

// assertOneLegacyBillingRead checks that the read sent exactly one GET, with
// no body, to wantPath, and nothing to a REST v1 path. It returns the query.
func assertOneLegacyBillingRead(t *testing.T, recorder *legacyBillingRecorder, wantPath string) url.Values {
	t.Helper()
	requests := recorder.all()
	for _, request := range requests {
		if strings.Contains(request.path, "/v1/") {
			t.Errorf("a request reached a REST v1 path: %s %s", request.method, request.path)
		}
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %+v, want exactly one", requests)
	}
	request := requests[0]
	if request.method != http.MethodGet || request.path != wantPath {
		t.Errorf("request = %s %s, want GET %s", request.method, request.path, wantPath)
	}
	if request.body != "" {
		t.Errorf("request body = %q, want none", request.body)
	}
	query, err := url.ParseQuery(request.rawQuery)
	if err != nil {
		t.Fatalf("query %q: %v", request.rawQuery, err)
	}
	return query
}

// legacyBillingConfig builds a data source's schema and a configuration in
// which every attribute not in config is null.
func legacyBillingConfig(ctx context.Context, t *testing.T, d datasource.DataSource, config map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
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
	for name, value := range config {
		values[name] = value
	}
	return tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, values)}
}

// readLegacyBilling drives a data source's Read the way Terraform does and
// returns the response, errors included.
func readLegacyBilling(ctx context.Context, t *testing.T, d datasource.DataSource, config map[string]tftypes.Value) *datasource.ReadResponse {
	t.Helper()
	cfg := legacyBillingConfig(ctx, t, d, config)
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: cfg.Schema, Raw: tftypes.NewValue(cfg.Raw.Type(), nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	return resp
}

func legacyBillingString(value string) tftypes.Value { return tftypes.NewValue(tftypes.String, value) }

// TestLegacyBillingReadsRESTv2 drives each deprecated billing data source
// against a stub REST v2 API: the configured window and filter have to reach
// the v2 route as its query parameters, end_time with a start 30 days before
// it (REST v2 takes the two only as a pair), nothing may reach a REST v1 path,
// and the v2 record has to come back in the 1.x record shape with the fields
// v2 lacks null. No GraphQL client is configured here, so the endpoint
// record's billed time stays null too.
func TestLegacyBillingReadsRESTv2(t *testing.T) {
	ctx := context.Background()
	window := map[string]tftypes.Value{
		"bucket_size": legacyBillingString("hour"),
		"end_time":    legacyBillingString("2026-06-02T00:00:00Z"),
	}
	withFilter := func(name, id string) map[string]tftypes.Value {
		config := map[string]tftypes.Value{name: legacyBillingString(id)}
		for key, value := range window {
			config[key] = value
		}
		return config
	}

	for _, testCase := range []struct {
		name        string
		dataSource  func(*runpod.Client) datasource.DataSource
		config      map[string]tftypes.Value
		body        string
		wantPath    string
		filterParam string
		filterValue string
		check       func(t *testing.T, state tfsdk.State)
	}{
		{
			name:       "runpod_billing_pod",
			dataSource: func(c *runpod.Client) datasource.DataSource { return &legacyPodBillingDataSource{client: c} },
			config:     withFilter("pod_id", "pod_abc"),
			body: `{"metadata":{"query":{"startTime":"2026-05-03T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"hour","podId":"pod_abc"},` +
				`"recordCount":1,"uniquePodCount":1,"totals":{"gpuAmount":1,"cpuAmount":0,"diskAmount":0.25,"totalAmount":1.25}},` +
				`"records":[{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-01T01:00:00Z","podId":"pod_abc",` +
				`"gpuAmount":1,"cpuAmount":0,"diskAmount":0.25,"totalAmount":1.25}]}`,
			wantPath:    "/v2/billing/pods",
			filterParam: "podId",
			filterValue: "pod_abc",
			check: func(t *testing.T, state tfsdk.State) {
				var model legacyPodBillingModel
				if diags := state.Get(ctx, &model); diags.HasError() {
					t.Fatal(diags)
				}
				if model.PodId.ValueString() != "pod_abc" || model.BucketSize.ValueString() != "hour" ||
					model.EndTime.ValueString() != "2026-06-02T00:00:00Z" {
					t.Errorf("configured attributes not echoed: %+v", model)
				}
				if len(model.BillingRecords) != 1 {
					t.Fatalf("records = %+v, want one", model.BillingRecords)
				}
				record := model.BillingRecords[0]
				if record.Amount.ValueFloat64() != 1.25 || record.Time.ValueString() != "2026-06-01T00:00:00Z" ||
					record.PodId.ValueString() != "pod_abc" {
					t.Errorf("record = %+v", record)
				}
				if !record.GpuTypeId.IsNull() || !record.EndpointId.IsNull() ||
					!record.DiskSpaceBilledGb.IsNull() || !record.TimeBilledMs.IsNull() {
					t.Errorf("fields REST v2 lacks must read null: %+v", record)
				}
			},
		},
		{
			name:       "runpod_billing_network_volume",
			dataSource: func(c *runpod.Client) datasource.DataSource { return &legacyNetworkVolumeBillingDataSource{client: c} },
			config:     withFilter("network_volume_id", "nv_abc"),
			body: `{"metadata":{"query":{"startTime":"2026-05-03T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"hour","networkVolumeId":"nv_abc"},` +
				`"recordCount":1,"uniqueNetworkVolumeCount":1,"totals":{"standardAmount":0.5,"highPerformanceAmount":0,"totalAmount":0.5}},` +
				`"records":[{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-01T01:00:00Z","networkVolumeId":"nv_abc",` +
				`"standardAmount":0.5,"highPerformanceAmount":0,"totalAmount":0.5}]}`,
			wantPath:    "/v2/billing/network-volumes",
			filterParam: "networkVolumeId",
			filterValue: "nv_abc",
			check: func(t *testing.T, state tfsdk.State) {
				var model legacyNetworkVolumeBillingModel
				if diags := state.Get(ctx, &model); diags.HasError() {
					t.Fatal(diags)
				}
				if model.NetworkVolumeId.ValueString() != "nv_abc" || model.EndTime.ValueString() != "2026-06-02T00:00:00Z" {
					t.Errorf("configured attributes not echoed: %+v", model)
				}
				if len(model.BillingRecords) != 1 {
					t.Fatalf("records = %+v, want one", model.BillingRecords)
				}
				record := model.BillingRecords[0]
				if record.Amount.ValueFloat64() != 0.5 || record.Time.ValueString() != "2026-06-01T00:00:00Z" ||
					record.NetworkVolumeId.ValueString() != "nv_abc" {
					t.Errorf("record = %+v", record)
				}
				if !record.DiskSpaceBilledGb.IsNull() || !record.TimeBilledMs.IsNull() {
					t.Errorf("fields REST v2 lacks must read null: %+v", record)
				}
			},
		},
		{
			// Serverless endpoint spend is /v2/billing/serverless; the
			// similarly named /v2/billing/endpoints is public endpoint spend
			// and has no endpoint filter.
			name:       "runpod_billing_endpoint",
			dataSource: func(c *runpod.Client) datasource.DataSource { return &legacyEndpointBillingDataSource{client: c} },
			config:     withFilter("endpoint_id", "ep_abc"),
			body: `{"metadata":{"query":{"startTime":"2026-05-03T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"hour","serverlessId":"ep_abc"},` +
				`"recordCount":1,"uniqueServerlessCount":1,"totals":{"gpuAmount":2,"cpuAmount":0,"diskAmount":0,"feeAmount":0,"totalAmount":2}},` +
				`"records":[{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-01T01:00:00Z","serverlessId":"ep_abc",` +
				`"gpuAmount":2,"cpuAmount":0,"diskAmount":0,"feeAmount":0,"totalAmount":2}]}`,
			wantPath:    "/v2/billing/serverless",
			filterParam: "serverlessId",
			filterValue: "ep_abc",
			check: func(t *testing.T, state tfsdk.State) {
				var model legacyEndpointBillingModel
				if diags := state.Get(ctx, &model); diags.HasError() {
					t.Fatal(diags)
				}
				if model.EndpointId.ValueString() != "ep_abc" || model.EndTime.ValueString() != "2026-06-02T00:00:00Z" {
					t.Errorf("configured attributes not echoed: %+v", model)
				}
				if len(model.BillingRecords) != 1 {
					t.Fatalf("records = %+v, want one", model.BillingRecords)
				}
				record := model.BillingRecords[0]
				if record.Amount.ValueFloat64() != 2 || record.Time.ValueString() != "2026-06-01T00:00:00Z" ||
					record.EndpointId.ValueString() != "ep_abc" {
					t.Errorf("record = %+v", record)
				}
				if !record.TimeBilledMs.IsNull() {
					t.Errorf("time_billed_ms must read null: %+v", record)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client, recorder := legacyBillingStub(t, http.StatusOK, testCase.body)
			resp := readLegacyBilling(ctx, t, testCase.dataSource(client), testCase.config)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}

			query := assertOneLegacyBillingRead(t, recorder, testCase.wantPath)
			want := url.Values{
				"bucketSize":         {"hour"},
				"startTime":          {"2026-05-03T00:00:00Z"},
				"endTime":            {"2026-06-02T00:00:00Z"},
				testCase.filterParam: {testCase.filterValue},
			}
			if query.Encode() != want.Encode() {
				t.Errorf("query = %s, want %s (no lastN)", query.Encode(), want.Encode())
			}
			testCase.check(t, resp.State)
		})
	}
}

func TestLegacyBillingUnconfiguredReadSendsNoParameters(t *testing.T) {
	// data "runpod_billing_endpoint" "all" {}: REST v2 picks the window and
	// bucket, and no empty filter goes out. An account with no spend reads an
	// empty list rather than null.
	ctx := context.Background()
	client, recorder := legacyBillingStub(t, http.StatusOK,
		`{"metadata":{"query":{"startTime":"2026-05-03T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"day"},`+
			`"recordCount":0,"uniqueServerlessCount":0,"totals":{"totalAmount":0}},"records":[]}`)

	resp := readLegacyBilling(ctx, t, &legacyEndpointBillingDataSource{client: client}, map[string]tftypes.Value{
		"endpoint_id": legacyBillingString(""),
	})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if query := assertOneLegacyBillingRead(t, recorder, "/v2/billing/serverless"); len(query) != 0 {
		t.Errorf("query = %s, want none", query.Encode())
	}

	var model legacyEndpointBillingModel
	if diags := resp.State.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.BillingRecords == nil || len(model.BillingRecords) != 0 {
		t.Errorf("billing_records = %+v, want an empty list", model.BillingRecords)
	}
}

func TestLegacyBillingWindowEndTimeSpellings(t *testing.T) {
	// 1.x documented end_time as "an ISO datetime" and passed it through;
	// REST v2 takes RFC 3339 only.
	for _, testCase := range []struct {
		value string
		want  time.Time
	}{
		{"2026-06-02T00:00:00Z", time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)},
		{"2026-06-02T03:00:00+03:00", time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)},
		{"2026-06-02T00:00:00", time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)},
		{"2026-06-02", time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)},
	} {
		var diags diag.Diagnostics
		window := legacyBillingWindow(types.StringValue("week"), types.StringValue(testCase.value), &diags)
		if diags.HasError() {
			t.Errorf("%q: %v", testCase.value, diags)
			continue
		}
		if window.end == nil || !window.end.Equal(testCase.want) {
			t.Errorf("%q: end = %v, want %v", testCase.value, window.end, testCase.want)
		}
		// REST v2 answers a lone endTime with a 400, so the start goes out
		// with it, the 30 days REST v2 defaults to.
		if window.start == nil || !window.start.Equal(testCase.want.Add(-30*24*time.Hour)) {
			t.Errorf("%q: start = %v, want 30 days before the end", testCase.value, window.start)
		}
		if window.bucket == nil || *window.bucket != sdktypes.BillingBucketSizeWeek {
			t.Errorf("%q: bucket = %v", testCase.value, window.bucket)
		}
	}

	var diags diag.Diagnostics
	if window := legacyBillingWindow(types.StringNull(), types.StringValue("June 2nd"), &diags); !diags.HasError() || window.start != nil || window.end != nil {
		t.Errorf("an unparsable end_time must be an attribute error: window = %+v, diags = %v", window, diags)
	}

	diags = nil
	window := legacyBillingWindow(types.StringNull(), types.StringNull(), &diags)
	if diags.HasError() || window.bucket != nil || window.start != nil || window.end != nil {
		t.Errorf("an unset window must send nothing: window = %+v, diags = %v", window, diags)
	}
}

func TestLegacyPodBillingRejectsTheGPUTypeWithAPod(t *testing.T) {
	// The GPU type view comes from GraphQL, which has no pod filter, and the
	// pod view from REST v2, which has no GPU type: the two filters together
	// have no source.
	ctx := context.Background()
	d := &legacyPodBillingDataSource{}
	gpu := legacyBillingString("NVIDIA A100 80GB PCIe")
	null := tftypes.NewValue(tftypes.String, nil)
	unknown := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

	for _, testCase := range []struct {
		name     string
		gpu, pod tftypes.Value
		wantErr  bool
	}{
		{"gpu type alone", gpu, null, false},
		{"pod alone", null, legacyBillingString("pod_abc"), false},
		{"both", gpu, legacyBillingString("pod_abc"), true},
		{"empty pod", gpu, legacyBillingString(""), false},
		{"unknown pod", gpu, unknown, false},
	} {
		var resp datasource.ValidateConfigResponse
		d.ValidateConfig(ctx, datasource.ValidateConfigRequest{
			Config: legacyBillingConfig(ctx, t, d, map[string]tftypes.Value{"gpu_type_id": testCase.gpu, "pod_id": testCase.pod}),
		}, &resp)
		if resp.Diagnostics.HasError() != testCase.wantErr {
			t.Errorf("%s: diagnostics = %v, want error %v", testCase.name, resp.Diagnostics, testCase.wantErr)
		}
	}

	client, recorder := legacyBillingStub(t, http.StatusOK, `{}`)
	resp := readLegacyBilling(ctx, t, &legacyPodBillingDataSource{client: client}, map[string]tftypes.Value{
		"gpu_type_id": gpu,
		"pod_id":      legacyBillingString("pod_abc"),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("a read with both filters set must error")
	}
	if requests := recorder.all(); len(requests) != 0 {
		t.Errorf("a rejected pair must send nothing, sent %+v", requests)
	}
}

func TestLegacyPodBillingByGPUTypeReadsGraphQL(t *testing.T) {
	// REST v1 grouped pod billing by GPU type: a gpu_type_id read answers that
	// way, from the GraphQL GPU pod billing, one record per bucket for that
	// type, with nothing sent to REST v2.
	ctx := context.Background()
	v2, v2Recorder := legacyBillingStub(t, http.StatusOK, `{}`)
	graphQL, graphQLRecorder := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"myself":{"billing":{"gpuCloud":[` +
			`{"gpuTypeId":"NVIDIA A100 80GB PCIe","time":"2026-06-01T00:00:00.000Z","timeBilledSeconds":3600,"amount":1.5},` +
			`{"gpuTypeId":"NVIDIA GeForce RTX 4090","time":"2026-06-01T00:00:00.000Z","timeBilledSeconds":60,"amount":0.1},` +
			`{"gpuTypeId":"NVIDIA A100 80GB PCIe","time":"2026-05-31T00:00:00.000Z","timeBilledSeconds":7200,"amount":3}]}}}}`
	})
	d := &legacyPodBillingDataSource{
		client: v2,
		legacy: graphQL,
		now:    func() time.Time { return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC) },
	}

	resp := readLegacyBilling(ctx, t, d, map[string]tftypes.Value{
		"gpu_type_id": legacyBillingString("NVIDIA A100 80GB PCIe"),
		"end_time":    legacyBillingString("2026-06-02T00:00:00Z"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if requests := v2Recorder.all(); len(requests) != 0 {
		t.Errorf("a gpu_type_id read must not reach REST v2, sent %+v", requests)
	}
	assertOnlyGraphQL(t, graphQLRecorder)
	calls := graphQLRecorder.Calls()
	if len(calls) != 1 || calls[0].Query != legacyGPUCloudBillingQuery {
		t.Fatalf("calls = %+v, want one legacyGPUCloudBillingQuery", calls)
	}
	input, ok := calls[0].Variables["input"].(map[string]any)
	if !ok || len(calls[0].Variables) != 1 || len(input) != 3 || input["granularity"] != "DAILY" ||
		input["startDate"] != "2026-05-03T00:00:00Z" || input["endDate"] != "2026-06-02T00:00:00Z" {
		t.Errorf("variables = %v, want input {DAILY, 2026-05-03T00:00:00Z, 2026-06-02T00:00:00Z}", calls[0].Variables)
	}

	var model legacyPodBillingModel
	if diags := resp.State.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if len(model.BillingRecords) != 2 {
		t.Fatalf("records = %+v, want the two A100 buckets", model.BillingRecords)
	}
	first, second := model.BillingRecords[0], model.BillingRecords[1]
	if first.Time.ValueString() != "2026-05-31T00:00:00Z" || first.Amount.ValueFloat64() != 3 ||
		first.TimeBilledMs.ValueInt64() != 7200000 || first.GpuTypeId.ValueString() != "NVIDIA A100 80GB PCIe" {
		t.Errorf("first record = %+v", first)
	}
	if second.Time.ValueString() != "2026-06-01T00:00:00Z" || second.Amount.ValueFloat64() != 1.5 ||
		second.TimeBilledMs.ValueInt64() != 3600000 {
		t.Errorf("second record = %+v", second)
	}
	if !first.PodId.IsNull() || !first.EndpointId.IsNull() || !first.DiskSpaceBilledGb.IsNull() {
		t.Errorf("a GPU type record has no pod, endpoint or billed disk: %+v", first)
	}
}

func TestLegacyPodBillingByGPUTypeSumsMonthsIntoYears(t *testing.T) {
	// GraphQL has no yearly granularity: a yearly bucket sums the monthly
	// rows of its year.
	ctx := context.Background()
	graphQL, graphQLRecorder := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"myself":{"billing":{"gpuCloud":[` +
			`{"gpuTypeId":"NVIDIA A100 80GB PCIe","time":"2026-05-01T00:00:00.000Z","timeBilledSeconds":10,"amount":1},` +
			`{"gpuTypeId":"NVIDIA A100 80GB PCIe","time":"2026-06-01T00:00:00.000Z","timeBilledSeconds":20,"amount":2}]}}}}`
	})
	d := &legacyPodBillingDataSource{
		legacy: graphQL,
		now:    func() time.Time { return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC) },
	}

	resp := readLegacyBilling(ctx, t, d, map[string]tftypes.Value{
		"gpu_type_id": legacyBillingString("NVIDIA A100 80GB PCIe"),
		"bucket_size": legacyBillingString("year"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	for _, call := range graphQLRecorder.Calls() {
		if input, _ := call.Variables["input"].(map[string]any); input["granularity"] != "MONTHLY" {
			t.Errorf("variables = %v, want MONTHLY for a yearly bucket", call.Variables)
		}
	}
	var model legacyPodBillingModel
	if diags := resp.State.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	// The rows come back once per piece the stub answers, so the year sums
	// them once per GraphQL call.
	calls := len(graphQLRecorder.Calls())
	if len(model.BillingRecords) != 1 || model.BillingRecords[0].Time.ValueString() != "2026-01-01T00:00:00Z" ||
		model.BillingRecords[0].Amount.ValueFloat64() != float64(3*calls) ||
		model.BillingRecords[0].TimeBilledMs.ValueInt64() != int64(30000*calls) {
		t.Errorf("records = %+v, want one 2026 record summing May and June", model.BillingRecords)
	}
}

func TestLegacyPodBillingByGPUTypeFailsWhenGraphQLFails(t *testing.T) {
	// The records are the data source's whole body, so a failed read is an
	// error, not an empty list.
	ctx := context.Background()
	graphQL, _ := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"myself":{"billing":null}}}`
	})
	d := &legacyPodBillingDataSource{
		legacy: graphQL,
		now:    func() time.Time { return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC) },
	}
	resp := readLegacyBilling(ctx, t, d, map[string]tftypes.Value{"gpu_type_id": legacyBillingString("NVIDIA A100 80GB PCIe")})
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), errLegacyBillingWithheld.Error()) {
		t.Fatalf("diagnostics = %v, want an error naming the withheld billing", resp.Diagnostics)
	}
}

func TestLegacyBillingErrorsWhenTheRouteFails(t *testing.T) {
	// The records are the whole body of these data sources and REST v2 is
	// their only source, so a failed read errors rather than writing
	// billing_records = null with a warning.
	ctx := context.Background()
	for _, testCase := range []struct {
		name       string
		dataSource func(*runpod.Client) datasource.DataSource
		status     int
		wantPath   string
	}{
		{"pod 500", func(c *runpod.Client) datasource.DataSource { return &legacyPodBillingDataSource{client: c} },
			http.StatusInternalServerError, "/v2/billing/pods"},
		{"network volume 403", func(c *runpod.Client) datasource.DataSource { return &legacyNetworkVolumeBillingDataSource{client: c} },
			http.StatusForbidden, "/v2/billing/network-volumes"},
		{"endpoint 404", func(c *runpod.Client) datasource.DataSource { return &legacyEndpointBillingDataSource{client: c} },
			http.StatusNotFound, "/v2/billing/serverless"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client, recorder := legacyBillingStub(t, testCase.status, `{"error":"billing is down"}`)
			resp := readLegacyBilling(ctx, t, testCase.dataSource(client), nil)

			assertOneLegacyBillingRead(t, recorder, testCase.wantPath)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("a failed billing read must error, not warn: %v", resp.Diagnostics)
			}
			detail := resp.Diagnostics.Errors()[0].Detail()
			if !strings.Contains(detail, "billing is down") || !strings.Contains(detail, testCase.wantPath) {
				t.Errorf("the error must carry the cause and the route: %q", detail)
			}
			if !resp.State.Raw.IsNull() {
				t.Error("a failed read must not write a state whose billing_records are null")
			}
		})
	}
}

func TestLegacyBillingRecordsMarkTheFieldsRESTv2Lacks(t *testing.T) {
	// Every always-null record field says so in its description and carries
	// the deprecation; every field REST v2 fills carries neither; a field the
	// GraphQL shim fills carries the shim's deprecation and description.
	ctx := context.Background()
	for _, testCase := range []struct {
		dataSource  datasource.DataSource
		wantNull    []string
		wantFilled  []string
		wantGraphQL []string
	}{
		{&legacyPodBillingDataSource{}, []string{"disk_space_billed_gb", "endpoint_id"},
			[]string{"amount", "pod_id", "time"}, []string{"gpu_type_id", "time_billed_ms"}},
		{&legacyNetworkVolumeBillingDataSource{}, []string{"disk_space_billed_gb", "time_billed_ms"},
			[]string{"amount", "network_volume_id", "time"}, nil},
		{&legacyEndpointBillingDataSource{}, nil, []string{"amount", "endpoint_id", "time"}, []string{"time_billed_ms"}},
	} {
		var resp datasource.SchemaResponse
		testCase.dataSource.Schema(ctx, datasource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		if resp.Schema.DeprecationMessage == "" {
			t.Errorf("%T: the data source must stay deprecated", testCase.dataSource)
		}
		records, ok := resp.Schema.Attributes["billing_records"].(schema.ListNestedAttribute)
		if !ok {
			t.Fatalf("%T: billing_records is %T", testCase.dataSource, resp.Schema.Attributes["billing_records"])
		}
		attributes := records.NestedObject.Attributes
		if want := len(testCase.wantNull) + len(testCase.wantFilled) + len(testCase.wantGraphQL); len(attributes) != want {
			t.Errorf("%T: record attributes = %d, want %d", testCase.dataSource, len(attributes), want)
		}
		for _, name := range testCase.wantGraphQL {
			attribute := attributes[name]
			if attribute == nil || attribute.GetDeprecationMessage() != legacyShimDeprecation ||
				!strings.HasSuffix(attribute.GetDescription(), legacyShimDescriptionSuffix) {
				t.Errorf("%T: %s must be marked as served by the GraphQL shim: %+v", testCase.dataSource, name, attribute)
			}
		}
		for _, name := range testCase.wantNull {
			attribute := attributes[name]
			if attribute == nil || attribute.GetDeprecationMessage() != legacyBillingNullDeprecation ||
				!strings.HasSuffix(attribute.GetDescription(), legacyBillingNullDescriptionSuffix) {
				t.Errorf("%T: %s must be marked always null: %+v", testCase.dataSource, name, attribute)
			}
		}
		for _, name := range testCase.wantFilled {
			attribute := attributes[name]
			if attribute == nil || attribute.GetDeprecationMessage() != "" ||
				strings.Contains(attribute.GetDescription(), "Always null") {
				t.Errorf("%T: %s is filled from REST v2 and must not be marked null: %+v", testCase.dataSource, name, attribute)
			}
		}
	}
}

func TestFlattenLegacyBillingRecords(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	pods := flattenLegacyPodBilling([]sdktypes.PodBillingRecord{
		{StartTime: start, EndTime: end, PodId: "pod_a", GpuAmount: 1, DiskAmount: 0.5, TotalAmount: 1.5},
		{StartTime: end, EndTime: end.Add(time.Hour), PodId: "pod_b", TotalAmount: 0},
	})
	if len(pods) != 2 || pods[0].Amount.ValueFloat64() != 1.5 || pods[0].PodId.ValueString() != "pod_a" ||
		pods[0].Time.ValueString() != "2026-06-01T00:00:00Z" || pods[1].PodId.ValueString() != "pod_b" {
		t.Errorf("pod records = %+v", pods)
	}
	if pods[1].Amount.IsNull() || pods[1].Amount.ValueFloat64() != 0 {
		t.Errorf("a zero amount is a reported zero, not null: %+v", pods[1])
	}

	volumes := flattenLegacyNetworkVolumeBilling([]sdktypes.NetworkVolumeBillingRecord{
		{StartTime: start, EndTime: end, NetworkVolumeId: "nv_a", StandardAmount: 0.25, HighPerformanceAmount: 0.5, TotalAmount: 0.75},
	})
	if len(volumes) != 1 || volumes[0].Amount.ValueFloat64() != 0.75 || volumes[0].NetworkVolumeId.ValueString() != "nv_a" ||
		!volumes[0].DiskSpaceBilledGb.IsNull() || !volumes[0].TimeBilledMs.IsNull() {
		t.Errorf("network volume records = %+v", volumes)
	}

	endpoints := flattenLegacyEndpointBilling([]sdktypes.ServerlessBillingRecord{
		{StartTime: start, EndTime: end, ServerlessId: "ep_a", GpuAmount: 2, TotalAmount: 2},
	})
	if len(endpoints) != 1 || endpoints[0].Amount.ValueFloat64() != 2 || endpoints[0].EndpointId.ValueString() != "ep_a" ||
		!endpoints[0].TimeBilledMs.IsNull() {
		t.Errorf("endpoint records = %+v", endpoints)
	}

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

// legacyEndpointBillingV2Body is a REST v2 serverless billing answer for one
// day of hourly buckets: ep_a at 00:00 and 01:00, ep_b at 00:00.
const legacyEndpointBillingV2Body = `{"metadata":{"query":{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-02T00:00:00Z","bucketSize":"hour"},` +
	`"recordCount":3,"uniqueServerlessCount":2,"totals":{"gpuAmount":6,"cpuAmount":0,"diskAmount":0,"feeAmount":0,"totalAmount":6}},` +
	`"records":[` +
	`{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-01T01:00:00Z","serverlessId":"ep_a","gpuAmount":2,"cpuAmount":0,"diskAmount":0,"feeAmount":0,"totalAmount":2},` +
	`{"startTime":"2026-06-01T01:00:00Z","endTime":"2026-06-01T02:00:00Z","serverlessId":"ep_a","gpuAmount":1,"cpuAmount":0,"diskAmount":0,"feeAmount":0,"totalAmount":1},` +
	`{"startTime":"2026-06-01T00:00:00Z","endTime":"2026-06-01T01:00:00Z","serverlessId":"ep_b","gpuAmount":3,"cpuAmount":0,"diskAmount":0,"feeAmount":0,"totalAmount":3}]}`

func TestLegacyEndpointBillingFillsTheBilledTimeFromGraphQL(t *testing.T) {
	// REST v2 has no billed time; the GraphQL API's serverless billing grouped
	// by endpoint has it per endpoint and bucket. Each REST v2 record takes
	// the row with its endpoint and bucket start, in milliseconds; a record
	// with no row keeps null.
	ctx := context.Background()
	v2, v2Recorder := legacyBillingStub(t, http.StatusOK, legacyEndpointBillingV2Body)
	graphQL, graphQLRecorder := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"myself":{"billing":{"serverless":[` +
			`{"endpointId":"ep_a","time":"2026-06-01T00:00:00.000Z","timeBilledSeconds":90},` +
			`{"endpointId":"ep_b","time":"2026-06-01T00:00:00.000Z","timeBilledSeconds":30},` +
			`{"endpointId":"ep_c","time":"2026-06-01T00:00:00.000Z","timeBilledSeconds":7}]}}}}`
	})
	d := &legacyEndpointBillingDataSource{
		client: v2,
		legacy: graphQL,
		now:    func() time.Time { return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC) },
	}

	resp := readLegacyBilling(ctx, t, d, map[string]tftypes.Value{"bucket_size": legacyBillingString("hour")})
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
		t.Fatal(resp.Diagnostics)
	}

	if query := assertOneLegacyBillingRead(t, v2Recorder, "/v2/billing/serverless"); query.Encode() != "bucketSize=hour" {
		t.Errorf("REST v2 query = %s, want bucketSize=hour", query.Encode())
	}
	assertOnlyGraphQL(t, graphQLRecorder)
	calls := graphQLRecorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("GraphQL calls = %+v, want one", calls)
	}
	if calls[0].Query != legacyServerlessBilledTimeQuery {
		t.Errorf("document = %q, want legacyServerlessBilledTimeQuery", calls[0].Query)
	}
	wantInput := map[string]any{"granularity": "HOURLY", "startDate": "2026-06-01T00:00:00Z", "endDate": "2026-06-02T00:00:00Z"}
	if input, ok := calls[0].Variables["input"].(map[string]any); !ok || len(calls[0].Variables) != 1 ||
		len(input) != len(wantInput) || input["granularity"] != wantInput["granularity"] ||
		input["startDate"] != wantInput["startDate"] || input["endDate"] != wantInput["endDate"] {
		t.Errorf("variables = %v, want input %v (the window REST v2 resolved)", calls[0].Variables, wantInput)
	}

	var model legacyEndpointBillingModel
	if diags := resp.State.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if len(model.BillingRecords) != 3 {
		t.Fatalf("records = %+v, want three", model.BillingRecords)
	}
	if got := model.BillingRecords[0].TimeBilledMs; got.ValueInt64() != 90000 {
		t.Errorf("ep_a 00:00 time_billed_ms = %v, want 90000", got)
	}
	if got := model.BillingRecords[1].TimeBilledMs; !got.IsNull() {
		t.Errorf("ep_a 01:00 has no GraphQL row, time_billed_ms = %v, want null", got)
	}
	if got := model.BillingRecords[2].TimeBilledMs; got.ValueInt64() != 30000 {
		t.Errorf("ep_b 00:00 time_billed_ms = %v, want 30000", got)
	}
}

func TestLegacyEndpointBillingWarnsWhenTheBilledTimeReadFails(t *testing.T) {
	// The REST v2 records are the data source's body, so a failed GraphQL
	// read only warns, and every billed time stays null.
	ctx := context.Background()
	v2, _ := legacyBillingStub(t, http.StatusOK, legacyEndpointBillingV2Body)
	graphQL, graphQLRecorder := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"errors":[{"message":"billing is unavailable"}]}`
	})
	d := &legacyEndpointBillingDataSource{
		client: v2,
		legacy: graphQL,
		now:    func() time.Time { return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC) },
	}

	resp := readLegacyBilling(ctx, t, d, map[string]tftypes.Value{"bucket_size": legacyBillingString("hour")})
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 1 {
		t.Fatalf("diagnostics = %v, want one warning and no error", resp.Diagnostics)
	}
	if !strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), "billing is unavailable") {
		t.Errorf("warning = %q, want the GraphQL message", resp.Diagnostics.Warnings()[0].Detail())
	}
	assertOnlyGraphQL(t, graphQLRecorder)

	var model legacyEndpointBillingModel
	if diags := resp.State.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if len(model.BillingRecords) != 3 {
		t.Fatalf("records = %+v, want three", model.BillingRecords)
	}
	for _, record := range model.BillingRecords {
		if !record.TimeBilledMs.IsNull() {
			t.Errorf("time_billed_ms = %v, want null after a failed read", record.TimeBilledMs)
		}
	}
}

func TestLegacyBilledTimeWindows(t *testing.T) {
	now := time.Date(2026, 6, 20, 10, 30, 0, 0, time.UTC)
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

	for _, testCase := range []struct {
		name            string
		bucket          sdktypes.BillingBucketSize
		start, end      time.Time
		wantGranularity string
		want            []legacyBilledTimeWindow
	}{
		{
			// Hourly rows are per hour only for the last 14 days: the read
			// starts 13 days before today, and older records keep null.
			name: "hour", bucket: sdktypes.BillingBucketSizeHour,
			start: day(2026, 5, 21), end: time.Date(2026, 6, 20, 11, 0, 0, 0, time.UTC),
			wantGranularity: "HOURLY",
			want:            []legacyBilledTimeWindow{{day(2026, 6, 7), time.Date(2026, 6, 20, 11, 0, 0, 0, time.UTC)}},
		},
		{
			// WEEKLY takes at most 30 days: six weeks go in 28-day pieces
			// that start on Mondays.
			name: "week", bucket: sdktypes.BillingBucketSizeWeek,
			start: day(2026, 5, 18), end: day(2026, 6, 29),
			wantGranularity: "WEEKLY",
			want:            []legacyBilledTimeWindow{{day(2026, 5, 18), day(2026, 6, 15)}, {day(2026, 6, 15), day(2026, 6, 29)}},
		},
		{
			// Clamped to 13 months back (2025-05-01, a Thursday) and rounded
			// up to the next Monday, so no week is read in part.
			name: "week clamped", bucket: sdktypes.BillingBucketSizeWeek,
			start: day(2025, 3, 3), end: day(2025, 6, 2),
			wantGranularity: "WEEKLY",
			want:            []legacyBilledTimeWindow{{day(2025, 5, 5), day(2025, 6, 2)}},
		},
		{
			name: "month", bucket: sdktypes.BillingBucketSizeMonth,
			start: day(2026, 5, 1), end: day(2026, 7, 1),
			wantGranularity: "MONTHLY",
			want:            []legacyBilledTimeWindow{{day(2026, 5, 1), day(2026, 7, 1)}},
		},
		{
			// Entirely older than 13 months: nothing is read.
			name: "day too old", bucket: sdktypes.BillingBucketSizeDay,
			start: day(2024, 1, 1), end: day(2024, 1, 31),
			wantGranularity: "DAILY",
		},
		{
			// GraphQL has no yearly granularity.
			name: "year", bucket: sdktypes.BillingBucketSizeYear,
			start: day(2026, 1, 1), end: day(2027, 1, 1),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			granularity, windows := legacyBilledTimeWindows(testCase.bucket, testCase.start, testCase.end, now)
			if granularity != testCase.wantGranularity {
				t.Errorf("granularity = %q, want %q", granularity, testCase.wantGranularity)
			}
			if len(windows) != len(testCase.want) {
				t.Fatalf("windows = %v, want %v", windows, testCase.want)
			}
			for i, window := range windows {
				if !window.start.Equal(testCase.want[i].start) || !window.end.Equal(testCase.want[i].end) {
					t.Errorf("window %d = %v, want %v", i, window, testCase.want[i])
				}
			}
		})
	}
}
