package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
	"github.com/runpod/go-sdk/v2/lib/billing"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// The three deprecated billing data sources the 1.x provider shipped:
// runpod_billing_pod, runpod_billing_network_volume and
// runpod_billing_endpoint. They exist so a 1.0.8 configuration keeps planning;
// every attribute name, filter and record field is the one that release
// declared. The records come from the REST v2 billing routes, mapped back into
// the 1.x record shape.
//
// REST v2 billing does not report four of the 1.x per-record fields
// (gpuTypeId, endpointId, diskSpaceBilledGb, timeBilledMs). Two of them have
// another source. The GraphQL API's serverless billing grouped by endpoint
// (UserServerlessBilling) carries each endpoint's billed time per bucket, and
// runpod_billing_endpoint fills time_billed_ms from it. The GraphQL GPU pod
// billing (UserGpuCloudBilling) carries spend and billed time per GPU type and
// bucket, the grouping REST v1 used for pod billing by default, and
// runpod_billing_pod answers a gpu_type_id read from it. The rest read null:
// the GraphQL billing types report CPU time per CPU flavor
// (UserCpuCloudBilling) and storage for the whole account
// (UserStorageBilling), never per pod or per volume.
//
// runpod_billing_endpoint reported Serverless endpoint spend filtered by
// endpoint id. The REST v2 route for that is /v2/billing/serverless with
// serverlessId. /v2/billing/endpoints is spend on Runpod's own public
// endpoints, takes no endpoint filter and carries no endpoint id; it backs
// runpod_endpoint_billing.
//
// New configurations should use runpod_pod_billing,
// runpod_network_volume_billing and runpod_serverless_billing instead: the
// same routes, with the cost components broken out and the whole window
// exposed.
//
// One 1.0.8 behaviour is deliberately not reproduced: end_time was dropped
// from the state of the network volume and endpoint data sources, which
// Terraform rejects as an inconsistent result.

const legacyBillingDeprecation = "Reports REST v2 billing in the 1.x record shape, with the per-record fields no API " +
	"reports always null; use runpod_pod_billing / runpod_network_volume_billing / runpod_serverless_billing"

// legacyBillingNullDeprecation is the DeprecationMessage of a 1.x billing
// record field that neither the REST v2 billing routes nor the GraphQL
// billing API reports per pod or per volume.
const legacyBillingNullDeprecation = "Always null: no API reports it per record; remove references to it"

// legacyBillingNullDescriptionSuffix ends the Description of such a field.
const legacyBillingNullDescriptionSuffix = "Always null: neither REST v2 billing nor the GraphQL billing API reports " +
	"this value per pod or per volume. Kept so a 1.x configuration that references it still plans; removed in a " +
	"later major release."

// legacyBillingEndTimeLayouts are the end_time spellings accepted. 1.x
// documented "an ISO datetime" and passed it through unparsed; REST v2 takes
// RFC 3339 only. A date, or a datetime without a zone, is read as UTC, so the
// common ISO spellings in 1.x configurations keep working.
var legacyBillingEndTimeLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"}

// legacyBillingLookback is the window length REST v2 defaults to. REST v2
// takes startTime and endTime only as a pair and answers a lone endTime with
// a 400, so a configured end_time goes out with a start this far before it,
// the same 30 days the default window covers.
const legacyBillingLookback = 30 * 24 * time.Hour

// legacyBillingWindowAttributes is the query window 1.0.8 accepted: a bucket
// size and an end time.
func legacyBillingWindowAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"bucket_size": schema.StringAttribute{
			Optional:    true,
			Description: "Length of each time bucket: hour, day, week, month, or year. Defaults to day.",
			Validators: []validator.String{
				stringvalidator.OneOf("hour", "day", "week", "month", "year"),
			},
		},
		"end_time": schema.StringAttribute{
			Optional: true,
			Description: "End of the reported window, exclusive: an RFC 3339 timestamp, or a date or a datetime without " +
				"a zone, read as UTC. The window is the 30 days before end_time, or before the current time when " +
				"end_time is unset, widened by REST v2 to whole buckets.",
		},
	}
}

// legacyBillingRecordAttributes is the part of a record every route carries.
func legacyBillingRecordAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"amount": schema.Float64Attribute{Computed: true, Description: "Amount charged in USD: the REST v2 record's total amount."},
		"time":   schema.StringAttribute{Computed: true, Description: "Start of the bucket this record bills, as an RFC 3339 timestamp."},
	}
}

// legacyBillingNullInt64 and legacyBillingNullString declare a 1.x record
// field REST v2 does not report.
func legacyBillingNullInt64(description string) schema.Attribute {
	return schema.Int64Attribute{
		Computed:           true,
		Description:        description + " " + legacyBillingNullDescriptionSuffix,
		DeprecationMessage: legacyBillingNullDeprecation,
	}
}

func legacyBillingNullString(description string) schema.Attribute {
	return schema.StringAttribute{
		Computed:           true,
		Description:        description + " " + legacyBillingNullDescriptionSuffix,
		DeprecationMessage: legacyBillingNullDeprecation,
	}
}

// legacyBillingRecords wraps a route's record list in the shared list
// attribute, under the 1.0.8 name.
func legacyBillingRecords(resource string, attributes map[string]schema.Attribute) schema.Attribute {
	return schema.ListNestedAttribute{
		Computed: true,
		Description: fmt.Sprintf("One record per %s per time bucket, in the 1.x record shape. An account with no spend "+
			"in the window reads an empty list.", resource),
		NestedObject: schema.NestedAttributeObject{Attributes: withAttributes(legacyBillingRecordAttributes(), attributes)},
	}
}

// legacyBillingQueryWindow is the configured 1.x window as REST v2 query
// parameters; a nil field stays out of the request.
type legacyBillingQueryWindow struct {
	bucket     *sdktypes.BillingBucketSize
	start, end *time.Time
}

// legacyBillingWindow turns the configured 1.x window into REST v2 query
// parameters. Unset attributes stay out of the request, so REST v2 applies its
// own defaults, which match the ones 1.x documented. A configured end_time
// is sent with the start legacyBillingLookback before it.
func legacyBillingWindow(bucketSize, endTime types.String, diags *diag.Diagnostics) legacyBillingQueryWindow {
	var window legacyBillingQueryWindow
	if isSet(bucketSize) && bucketSize.ValueString() != "" {
		value := sdktypes.BillingBucketSize(bucketSize.ValueString())
		window.bucket = &value
	}
	if !isSet(endTime) || endTime.ValueString() == "" {
		return window
	}
	for _, layout := range legacyBillingEndTimeLayouts {
		if parsed, err := time.Parse(layout, endTime.ValueString()); err == nil {
			start := parsed.Add(-legacyBillingLookback)
			window.start, window.end = &start, &parsed
			return window
		}
	}
	diags.AddAttributeError(
		path.Root("end_time"),
		"Invalid end_time",
		fmt.Sprintf("end_time %q must be an RFC 3339 timestamp such as 2026-06-01T00:00:00Z, or a date such as "+
			"2026-06-01, which is read as midnight UTC.", endTime.ValueString()),
	)
	return window
}

// legacyBillingFilter is a configured resource filter as a query parameter.
// An empty string is no filter rather than a filter that matches nothing.
func legacyBillingFilter(value types.String) *string {
	if !isSet(value) || value.ValueString() == "" {
		return nil
	}
	return value.ValueStringPointer()
}

func legacyBillingTime(value time.Time) types.String {
	return types.StringValue(value.Format(time.RFC3339))
}

// appendLegacyBillingFailure records a billing read that did not answer.
// Every failure here is an error, filtered or not: the records are the whole
// body of these data sources and the REST v2 route is their only source.
// Answering with null records hands the first `billing_records[0]` an
// "Attempt to index null value" and leaves the real cause in a warning that
// has already scrolled past.
func appendLegacyBillingFailure(diags *diag.Diagnostics, subject, route string, err error) {
	diags.AddError(
		fmt.Sprintf("Reading the deprecated %s failed", subject),
		fmt.Sprintf(
			"%s\n\nThis deprecated data source reads GET %s and reports its records in the 1.x shape; it has no other source.",
			err.Error(), route,
		),
	)
}

// legacyBillingStatusError describes a response that carried no records.
func legacyBillingStatusError(response interface{ StatusCode() int }, body []byte) error {
	return fmt.Errorf("API returned status %d: %s", response.StatusCode(), boundedBody(body))
}

// rejectLegacyBillingFilterPair fails a configuration that sets both
// gpu_type_id and pod_id. The two views come from different APIs: the GPU
// type view from the GraphQL billing API, which reports spend per GPU type and
// has no pod filter, and the pod view from REST v2, which bills per pod and
// reports no GPU type.
func rejectLegacyBillingFilterPair(diags *diag.Diagnostics, gpuTypeID, podID types.String) {
	if legacyBillingFilter(gpuTypeID) == nil || legacyBillingFilter(podID) == nil {
		return
	}
	diags.AddAttributeError(
		path.Root("gpu_type_id"),
		"gpu_type_id and pod_id cannot be combined",
		"A gpu_type_id read comes from the GraphQL billing API, which reports spend per GPU type and cannot filter "+
			"by pod; a pod_id read comes from REST v2, which bills per pod and reports no GPU type. Set one of the two.",
	)
}

var (
	_ datasource.DataSource                   = (*legacyPodBillingDataSource)(nil)
	_ datasource.DataSourceWithConfigure      = (*legacyPodBillingDataSource)(nil)
	_ datasource.DataSourceWithValidateConfig = (*legacyPodBillingDataSource)(nil)
)

func NewLegacyPodBillingDataSource() datasource.DataSource { return &legacyPodBillingDataSource{} }

type legacyPodBillingDataSource struct {
	client *runpod.Client
	legacy *legacy.Client
	// now is the clock the GPU-type window is measured against; nil means
	// time.Now. Tests pin it.
	now func() time.Time
}

type legacyPodBillingRecordModel struct {
	Amount            types.Float64 `tfsdk:"amount"`
	DiskSpaceBilledGb types.Int64   `tfsdk:"disk_space_billed_gb"`
	EndpointId        types.String  `tfsdk:"endpoint_id"`
	GpuTypeId         types.String  `tfsdk:"gpu_type_id"`
	PodId             types.String  `tfsdk:"pod_id"`
	Time              types.String  `tfsdk:"time"`
	TimeBilledMs      types.Int64   `tfsdk:"time_billed_ms"`
}

type legacyPodBillingModel struct {
	BucketSize     types.String                  `tfsdk:"bucket_size"`
	EndTime        types.String                  `tfsdk:"end_time"`
	GpuTypeId      types.String                  `tfsdk:"gpu_type_id"`
	PodId          types.String                  `tfsdk:"pod_id"`
	BillingRecords []legacyPodBillingRecordModel `tfsdk:"billing_records"`
}

func (m legacyPodBillingModel) params(diags *diag.Diagnostics) *billing.ListPodBillingParams {
	window := legacyBillingWindow(m.BucketSize, m.EndTime, diags)
	return &billing.ListPodBillingParams{
		BucketSize: window.bucket, StartTime: window.start, EndTime: window.end, PodId: legacyBillingFilter(m.PodId),
	}
}

func (d *legacyPodBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing_pod"
}

func (d *legacyPodBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deprecated. Pod spend in the 1.x provider's record shape. Without gpu_type_id it reads the " +
			"REST v2 route GET /v2/billing/pods, one record per pod and bucket, with gpu_type_id, endpoint_id, " +
			"disk_space_billed_gb and time_billed_ms null. With gpu_type_id it answers the way REST v1 did, one " +
			"record per bucket for that GPU type, read from the GraphQL billing API. Use runpod_pod_billing, which " +
			"reads the REST v2 route with the cost components broken out.",
		DeprecationMessage: legacyBillingDeprecation,
		Attributes: withAttributes(legacyBillingWindowAttributes(), map[string]schema.Attribute{
			"gpu_type_id": schema.StringAttribute{
				Optional: true,
				Description: "Restrict the records to one GPU type. The read then comes from the GraphQL billing API's " +
					"GPU pod billing, which reports spend and billed time per GPU type and bucket, the way REST v1 " +
					"grouped pod billing: gpu_type_id, amount, time and time_billed_ms are filled, pod_id, endpoint_id " +
					"and disk_space_billed_gb are null. Hourly buckets more than 13 days old and other buckets more " +
					"than 13 months old are not reported, and a yearly bucket sums the monthly rows inside that reach. " +
					"Cannot be combined with pod_id. " + legacyShimDescriptionSuffix,
				DeprecationMessage: legacyShimDeprecation,
			},
			"pod_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one pod, read from REST v2."},
			"billing_records": legacyBillingRecords("pod", map[string]schema.Attribute{
				"disk_space_billed_gb": legacyBillingNullInt64("Disk space billed in GB."),
				"endpoint_id":          legacyBillingNullString("Endpoint the pod served, when it was a Serverless worker."),
				"gpu_type_id": schema.StringAttribute{
					Computed: true,
					Description: "GPU type the record bills: the gpu_type_id filter's value when it is set, null " +
						"otherwise, because REST v2 bills per pod without a GPU type. " + legacyShimDescriptionSuffix,
					DeprecationMessage: legacyShimDeprecation,
				},
				"pod_id": schema.StringAttribute{Computed: true, Description: "The pod this record bills; null when gpu_type_id is set."},
				"time_billed_ms": schema.Int64Attribute{
					Computed: true,
					Description: "Time billed in milliseconds, at whole-second precision, when gpu_type_id is set; " +
						"null otherwise, because no API reports billed time per pod. " + legacyShimDescriptionSuffix,
					DeprecationMessage: legacyShimDeprecation,
				},
			}),
		}),
	}
}

func (d *legacyPodBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
	d.legacy = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

// readLegacyPodBillingByGPUType answers a gpu_type_id read the way REST v1
// did. REST v1 grouped pod billing by GPU type unless asked otherwise, one
// record per GPU type and bucket, with no pod id. REST v2 bills per pod and
// cannot filter by GPU type, but the GraphQL billing API reports GPU pod spend
// and billed time per GPU type and bucket (UserBilling.gpuCloud), so the
// records come from there: gpu_type_id, amount, time and time_billed_ms
// filled; pod_id, endpoint_id and disk_space_billed_gb null. The window is the
// one REST v2 would use, widened to whole buckets; yearly buckets sum monthly
// rows, since GraphQL has no yearly granularity.
func (d *legacyPodBillingDataSource) readLegacyPodBillingByGPUType(ctx context.Context, model legacyPodBillingModel, gpuType string, diags *diag.Diagnostics) []legacyPodBillingRecordModel {
	const subject = "pod billing by GPU type"
	if d.legacy == nil {
		appendLegacyGPUTypeBillingFailure(diags, subject, errors.New("the provider has no GraphQL client configured"))
		return nil
	}
	window := legacyBillingWindow(model.BucketSize, model.EndTime, diags)
	if diags.HasError() {
		return nil
	}
	now := time.Now
	if d.now != nil {
		now = d.now
	}
	bucket := sdktypes.BillingBucketSizeDay
	if window.bucket != nil {
		bucket = *window.bucket
	}
	end := now().UTC()
	if window.end != nil {
		end = window.end.UTC()
	}
	start, end := legacyBucketFloor(end.Add(-legacyBillingLookback), bucket), legacyBucketCeil(end, bucket)

	readBucket := bucket
	if bucket == sdktypes.BillingBucketSizeYear {
		readBucket = sdktypes.BillingBucketSizeMonth
	}
	granularity, windows := legacyBilledTimeWindows(readBucket, start, end, now())

	type total struct {
		amount  float64
		seconds int64
	}
	totals := map[time.Time]*total{}
	for _, piece := range windows {
		rows, err := fetchLegacyGPUCloudBilling(ctx, d.legacy, granularity, piece.start, piece.end)
		if err != nil {
			appendLegacyGPUTypeBillingFailure(diags, subject, err)
			return nil
		}
		for _, row := range rows {
			if row.GpuTypeID == nil || *row.GpuTypeID != gpuType || row.Time == nil {
				continue
			}
			key := legacyBucketFloor(*row.Time, bucket)
			if totals[key] == nil {
				totals[key] = &total{}
			}
			if row.Amount != nil {
				totals[key].amount += *row.Amount
			}
			if row.TimeBilledSeconds != nil {
				totals[key].seconds += *row.TimeBilledSeconds
			}
		}
	}

	starts := make([]time.Time, 0, len(totals))
	for start := range totals {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	records := make([]legacyPodBillingRecordModel, 0, len(starts))
	for _, start := range starts {
		records = append(records, legacyPodBillingRecordModel{
			Amount:            types.Float64Value(totals[start].amount),
			DiskSpaceBilledGb: types.Int64Null(),
			EndpointId:        types.StringNull(),
			GpuTypeId:         types.StringValue(gpuType),
			PodId:             types.StringNull(),
			Time:              legacyBillingTime(start),
			TimeBilledMs:      types.Int64Value(totals[start].seconds * 1000),
		})
	}
	return records
}

// appendLegacyGPUTypeBillingFailure records a gpu_type_id read that did not
// answer. Like a REST v2 billing failure it is an error: the records are the
// data source's whole body.
func appendLegacyGPUTypeBillingFailure(diags *diag.Diagnostics, subject string, err error) {
	diags.AddError(
		fmt.Sprintf("Reading the deprecated %s failed", subject),
		fmt.Sprintf(
			"%s\n\nA gpu_type_id read comes from the GraphQL billing API (UserBilling.gpuCloud); it has no other source.",
			err.Error(),
		),
	)
}

// ValidateConfig rejects gpu_type_id together with pod_id at plan time,
// before any read. An unknown value is left to Read, which sees it resolved.
func (d *legacyPodBillingDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var gpuTypeID, podID types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("gpu_type_id"), &gpuTypeID)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("pod_id"), &podID)...)
	if resp.Diagnostics.HasError() || gpuTypeID.IsUnknown() || podID.IsUnknown() {
		return
	}
	rejectLegacyBillingFilterPair(&resp.Diagnostics, gpuTypeID, podID)
}

func (d *legacyPodBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state legacyPodBillingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rejectLegacyBillingFilterPair(&resp.Diagnostics, state.GpuTypeId, state.PodId)
	if resp.Diagnostics.HasError() {
		return
	}
	if gpuType := legacyBillingFilter(state.GpuTypeId); gpuType != nil {
		state.BillingRecords = d.readLegacyPodBillingByGPUType(ctx, state, *gpuType, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	params := state.params(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	const route = "/v2/billing/pods"
	apiResp, err := d.client.Billing.ListPodBillingWithResponse(ctx, params)
	if err != nil {
		appendLegacyBillingFailure(&resp.Diagnostics, "pod billing", route, err)
		return
	}
	if apiResp.JSON200 == nil {
		appendLegacyBillingFailure(&resp.Diagnostics, "pod billing", route, legacyBillingStatusError(apiResp, apiResp.Body))
		return
	}

	state.BillingRecords = flattenLegacyPodBilling(apiResp.JSON200.Records)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenLegacyPodBilling(records []sdktypes.PodBillingRecord) []legacyPodBillingRecordModel {
	models := make([]legacyPodBillingRecordModel, 0, len(records))
	for _, record := range records {
		models = append(models, legacyPodBillingRecordModel{
			Amount:            types.Float64Value(record.TotalAmount),
			DiskSpaceBilledGb: types.Int64Null(),
			EndpointId:        types.StringNull(),
			GpuTypeId:         types.StringNull(),
			PodId:             types.StringValue(record.PodId),
			Time:              legacyBillingTime(record.StartTime),
			TimeBilledMs:      types.Int64Null(),
		})
	}
	return models
}

var (
	_ datasource.DataSource              = (*legacyNetworkVolumeBillingDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*legacyNetworkVolumeBillingDataSource)(nil)
)

func NewLegacyNetworkVolumeBillingDataSource() datasource.DataSource {
	return &legacyNetworkVolumeBillingDataSource{}
}

type legacyNetworkVolumeBillingDataSource struct {
	client *runpod.Client
}

type legacyNetworkVolumeBillingRecordModel struct {
	Amount            types.Float64 `tfsdk:"amount"`
	DiskSpaceBilledGb types.Int64   `tfsdk:"disk_space_billed_gb"`
	NetworkVolumeId   types.String  `tfsdk:"network_volume_id"`
	Time              types.String  `tfsdk:"time"`
	TimeBilledMs      types.Int64   `tfsdk:"time_billed_ms"`
}

type legacyNetworkVolumeBillingModel struct {
	BucketSize      types.String                            `tfsdk:"bucket_size"`
	EndTime         types.String                            `tfsdk:"end_time"`
	NetworkVolumeId types.String                            `tfsdk:"network_volume_id"`
	BillingRecords  []legacyNetworkVolumeBillingRecordModel `tfsdk:"billing_records"`
}

func (m legacyNetworkVolumeBillingModel) params(diags *diag.Diagnostics) *billing.ListNetworkVolumeBillingParams {
	window := legacyBillingWindow(m.BucketSize, m.EndTime, diags)
	return &billing.ListNetworkVolumeBillingParams{
		BucketSize: window.bucket, StartTime: window.start, EndTime: window.end,
		NetworkVolumeId: legacyBillingFilter(m.NetworkVolumeId),
	}
}

func (d *legacyNetworkVolumeBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing_network_volume"
}

func (d *legacyNetworkVolumeBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deprecated. Network volume spend in the 1.x provider's record shape, read from the REST v2 route " +
			"GET /v2/billing/network-volumes. REST v2 does not report a record's billed disk space or billed time, so " +
			"disk_space_billed_gb and time_billed_ms are always null. Use runpod_network_volume_billing, which reads the " +
			"same route with standard and high-performance storage broken out.",
		DeprecationMessage: legacyBillingDeprecation,
		Attributes: withAttributes(legacyBillingWindowAttributes(), map[string]schema.Attribute{
			"network_volume_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one network volume."},
			"billing_records": legacyBillingRecords("network volume", map[string]schema.Attribute{
				"disk_space_billed_gb": legacyBillingNullInt64("Disk space billed in GB."),
				"network_volume_id":    schema.StringAttribute{Computed: true, Description: "The network volume this record bills."},
				"time_billed_ms":       legacyBillingNullInt64("Time billed in milliseconds."),
			}),
		}),
	}
}

func (d *legacyNetworkVolumeBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *legacyNetworkVolumeBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state legacyNetworkVolumeBillingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := state.params(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	const route = "/v2/billing/network-volumes"
	apiResp, err := d.client.Billing.ListNetworkVolumeBillingWithResponse(ctx, params)
	if err != nil {
		appendLegacyBillingFailure(&resp.Diagnostics, "network volume billing", route, err)
		return
	}
	if apiResp.JSON200 == nil {
		appendLegacyBillingFailure(&resp.Diagnostics, "network volume billing", route, legacyBillingStatusError(apiResp, apiResp.Body))
		return
	}

	state.BillingRecords = flattenLegacyNetworkVolumeBilling(apiResp.JSON200.Records)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenLegacyNetworkVolumeBilling(records []sdktypes.NetworkVolumeBillingRecord) []legacyNetworkVolumeBillingRecordModel {
	models := make([]legacyNetworkVolumeBillingRecordModel, 0, len(records))
	for _, record := range records {
		models = append(models, legacyNetworkVolumeBillingRecordModel{
			Amount:            types.Float64Value(record.TotalAmount),
			DiskSpaceBilledGb: types.Int64Null(),
			NetworkVolumeId:   types.StringValue(record.NetworkVolumeId),
			Time:              legacyBillingTime(record.StartTime),
			TimeBilledMs:      types.Int64Null(),
		})
	}
	return models
}

var (
	_ datasource.DataSource              = (*legacyEndpointBillingDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*legacyEndpointBillingDataSource)(nil)
)

func NewLegacyEndpointBillingDataSource() datasource.DataSource {
	return &legacyEndpointBillingDataSource{}
}

type legacyEndpointBillingDataSource struct {
	client *runpod.Client
	legacy *legacy.Client
	// now is the clock the billed-time window is measured against; nil means
	// time.Now. Tests pin it.
	now func() time.Time
}

type legacyEndpointBillingRecordModel struct {
	Amount       types.Float64 `tfsdk:"amount"`
	EndpointId   types.String  `tfsdk:"endpoint_id"`
	Time         types.String  `tfsdk:"time"`
	TimeBilledMs types.Int64   `tfsdk:"time_billed_ms"`
}

type legacyEndpointBillingModel struct {
	BucketSize     types.String                       `tfsdk:"bucket_size"`
	EndTime        types.String                       `tfsdk:"end_time"`
	EndpointId     types.String                       `tfsdk:"endpoint_id"`
	BillingRecords []legacyEndpointBillingRecordModel `tfsdk:"billing_records"`
}

func (m legacyEndpointBillingModel) params(diags *diag.Diagnostics) *billing.ListServerlessBillingParams {
	window := legacyBillingWindow(m.BucketSize, m.EndTime, diags)
	return &billing.ListServerlessBillingParams{
		BucketSize: window.bucket, StartTime: window.start, EndTime: window.end,
		ServerlessId: legacyBillingFilter(m.EndpointId),
	}
}

func (d *legacyEndpointBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing_endpoint"
}

func (d *legacyEndpointBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deprecated. Serverless endpoint spend in the 1.x provider's record shape, read from the REST v2 " +
			"route GET /v2/billing/serverless, which bills Serverless endpoints by id. REST v2 does not report a " +
			"record's billed time, so time_billed_ms comes from the GraphQL API's serverless billing for the same " +
			"endpoint and bucket. Use runpod_serverless_billing, which reads the same REST v2 route with the cost " +
			"components broken out. runpod_endpoint_billing is a different thing: spend on Runpod's public endpoints.",
		DeprecationMessage: legacyBillingDeprecation,
		Attributes: withAttributes(legacyBillingWindowAttributes(), map[string]schema.Attribute{
			"endpoint_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one Serverless endpoint."},
			"billing_records": legacyBillingRecords("Serverless endpoint", map[string]schema.Attribute{
				"endpoint_id": schema.StringAttribute{Computed: true, Description: "The Serverless endpoint this record bills."},
				"time_billed_ms": schema.Int64Attribute{
					Computed: true,
					Description: "Time billed in milliseconds, at whole-second precision, read from the GraphQL API's " +
						"serverless billing grouped by endpoint for the record's endpoint and bucket. Null for yearly " +
						"buckets, for hourly buckets more than 13 days old and other buckets more than 13 months old " +
						"(that API reports those only in coarser buckets), when it has no row for the bucket, and when " +
						"it does not answer. " + legacyShimDescriptionSuffix,
					DeprecationMessage: legacyShimDeprecation,
				},
			}),
		}),
	}
}

func (d *legacyEndpointBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
	d.legacy = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

func (d *legacyEndpointBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state legacyEndpointBillingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := state.params(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	const route = "/v2/billing/serverless"
	apiResp, err := d.client.Billing.ListServerlessBillingWithResponse(ctx, params)
	if err != nil {
		appendLegacyBillingFailure(&resp.Diagnostics, "endpoint billing", route, err)
		return
	}
	if apiResp.JSON200 == nil {
		appendLegacyBillingFailure(&resp.Diagnostics, "endpoint billing", route, legacyBillingStatusError(apiResp, apiResp.Body))
		return
	}

	records := apiResp.JSON200.Records
	state.BillingRecords = flattenLegacyEndpointBilling(records)
	resp.Diagnostics.Append(d.readLegacyEndpointBilledTime(ctx, apiResp.JSON200.Metadata.Query, records, state.BillingRecords)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// legacyBilledTimeKey matches a GraphQL billed-time row to a REST v2 record:
// the endpoint and the Unix second its bucket starts at.
type legacyBilledTimeKey struct {
	endpointID string
	start      int64
}

// readLegacyEndpointBilledTime fills time_billed_ms from the GraphQL API's
// serverless billing grouped by endpoint, for the window and bucket size REST
// v2 resolved. A record with no matching row keeps null. A failed read warns
// and leaves every record null: the REST v2 records are the data source's
// body and have already been read.
func (d *legacyEndpointBillingDataSource) readLegacyEndpointBilledTime(ctx context.Context, query sdktypes.ServerlessBillingQuery, records []sdktypes.ServerlessBillingRecord, models []legacyEndpointBillingRecordModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if d.legacy == nil || len(records) == 0 {
		return diags
	}
	now := time.Now
	if d.now != nil {
		now = d.now
	}
	granularity, windows := legacyBilledTimeWindows(query.BucketSize, query.StartTime, query.EndTime, now())

	billed := map[legacyBilledTimeKey]int64{}
	for _, window := range windows {
		rows, err := fetchLegacyServerlessBilledTime(ctx, d.legacy, granularity, window.start, window.end)
		if err != nil {
			appendLegacyReadFailure(&diags, "endpoint billing", err)
			return diags
		}
		for _, row := range rows {
			if row.EndpointID == nil || row.Time == nil || row.TimeBilledSeconds == nil {
				continue
			}
			billed[legacyBilledTimeKey{*row.EndpointID, row.Time.Unix()}] += *row.TimeBilledSeconds
		}
	}
	for i, record := range records {
		if seconds, ok := billed[legacyBilledTimeKey{record.ServerlessId, record.StartTime.Unix()}]; ok {
			models[i].TimeBilledMs = types.Int64Value(seconds * 1000)
		}
	}
	return diags
}

// legacyBilledTimeGranularities maps a REST v2 bucket size to the GraphQL
// BillingGranularity whose buckets start at the same UTC instants. GraphQL has
// no yearly granularity, so yearly records keep a null billed time.
var legacyBilledTimeGranularities = map[sdktypes.BillingBucketSize]string{
	sdktypes.BillingBucketSizeHour:  "HOURLY",
	sdktypes.BillingBucketSizeDay:   "DAILY",
	sdktypes.BillingBucketSizeWeek:  "WEEKLY",
	sdktypes.BillingBucketSizeMonth: "MONTHLY",
}

// legacyBilledTimeWindow is one [start, end) range read from User.billing.
type legacyBilledTimeWindow struct {
	start, end time.Time
}

// legacyBilledTimeWindows returns the GraphQL granularity for bucket and the
// windows to read for the REST v2 window [start, end).
//
// Two limits of User.billing shape them. It reports hourly rows only for about
// the last 14 days and daily, weekly and monthly rows only for about the last
// 14 months; an older part of a range comes back in coarser rows (a day or a
// month) stamped with the coarser bucket's start, which would match the wrong
// REST v2 record. So the read starts no earlier than 13 days (hourly) or 13
// months (otherwise) before now, rounded up to a whole bucket, and older
// records keep null. And UserBillingInput takes HOURLY and WEEKLY only for
// ranges of at most 30 days, so the window is read in 28-day pieces, which
// keep hours and weeks whole; daily and monthly pieces stay under its 12-month
// limit.
func legacyBilledTimeWindows(bucket sdktypes.BillingBucketSize, start, end, now time.Time) (string, []legacyBilledTimeWindow) {
	granularity, ok := legacyBilledTimeGranularities[bucket]
	if !ok {
		return "", nil
	}
	now = now.UTC()
	earliest := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -13, 0)
	piece := 180 * 24 * time.Hour
	switch bucket {
	case sdktypes.BillingBucketSizeHour:
		earliest = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -13)
		piece = 28 * 24 * time.Hour
	case sdktypes.BillingBucketSizeWeek:
		piece = 28 * 24 * time.Hour
	}
	start = start.UTC()
	if start.Before(earliest) {
		start = legacyBucketCeil(earliest, bucket)
	}

	var windows []legacyBilledTimeWindow
	for from := start; from.Before(end); from = from.Add(piece) {
		to := from.Add(piece)
		if to.After(end) {
			to = end
		}
		windows = append(windows, legacyBilledTimeWindow{from, to})
	}
	return granularity, windows
}

// legacyBucketFloor returns the start of the bucket t falls in. Buckets are
// UTC hours, days, Monday-start weeks, calendar months and years, the
// boundaries REST v2 and User.billing both use.
func legacyBucketFloor(t time.Time, bucket sdktypes.BillingBucketSize) time.Time {
	t = t.UTC()
	switch bucket {
	case sdktypes.BillingBucketSizeHour:
		return t.Truncate(time.Hour)
	case sdktypes.BillingBucketSizeWeek:
		midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return midnight.AddDate(0, 0, -((int(midnight.Weekday()) + 6) % 7))
	case sdktypes.BillingBucketSizeMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	case sdktypes.BillingBucketSizeYear:
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

// legacyBucketCeil returns t if it starts a bucket, else the start of the next
// one.
func legacyBucketCeil(t time.Time, bucket sdktypes.BillingBucketSize) time.Time {
	t = t.UTC()
	floor := legacyBucketFloor(t, bucket)
	if floor.Equal(t) {
		return t
	}
	switch bucket {
	case sdktypes.BillingBucketSizeHour:
		return floor.Add(time.Hour)
	case sdktypes.BillingBucketSizeWeek:
		return floor.AddDate(0, 0, 7)
	case sdktypes.BillingBucketSizeMonth:
		return floor.AddDate(0, 1, 0)
	case sdktypes.BillingBucketSizeYear:
		return floor.AddDate(1, 0, 0)
	default:
		return floor.AddDate(0, 0, 1)
	}
}

func flattenLegacyEndpointBilling(records []sdktypes.ServerlessBillingRecord) []legacyEndpointBillingRecordModel {
	models := make([]legacyEndpointBillingRecordModel, 0, len(records))
	for _, record := range records {
		models = append(models, legacyEndpointBillingRecordModel{
			Amount:       types.Float64Value(record.TotalAmount),
			EndpointId:   types.StringValue(record.ServerlessId),
			Time:         legacyBillingTime(record.StartTime),
			TimeBilledMs: types.Int64Null(),
		})
	}
	return models
}
