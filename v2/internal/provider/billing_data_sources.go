package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/billing"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// Billing data sources: runpod_billing plus one per billed resource kind.
// They replace the 1.x provider's billing data sources, which read the REST v1
// API; the v2 routes return time-bucketed records with the cost components
// broken out, and every query parameter is exposed as an optional attribute.
//
// Each data source reports the cost components of a record under `amounts`,
// the same shape as `metadata.totals`, so a record and the window total read
// the same way.
//
// The Serverless platform fee (serverlessFeeAmount on the account route,
// feeAmount on the Serverless route) is not mapped: the API deprecated it and
// always returns 0, with the charge folded into the compute amounts.

const billingReplacementNote = "Replaces the 1.x provider's billing data source, which read the REST v1 API."

// billingWindow is the query window shared by every billing route.
type billingWindow struct {
	StartTime  types.String
	EndTime    types.String
	BucketSize types.String
	LastN      types.Int64
}

type resolvedBillingWindow struct {
	StartTime  *sdktypes.BillingStartTime
	EndTime    *sdktypes.BillingEndTime
	BucketSize *sdktypes.BillingBucketSize
	LastN      *sdktypes.BillingLastN
}

func (w billingWindow) resolve() (resolvedBillingWindow, diag.Diagnostics) {
	var diags diag.Diagnostics
	var resolved resolvedBillingWindow

	if start, ok := parseRFC3339Attribute(w.StartTime, "start_time", &diags); ok {
		resolved.StartTime = &start
	}
	if end, ok := parseRFC3339Attribute(w.EndTime, "end_time", &diags); ok {
		resolved.EndTime = &end
	}
	if !w.BucketSize.IsNull() && !w.BucketSize.IsUnknown() {
		bucket := sdktypes.BillingBucketSize(w.BucketSize.ValueString())
		resolved.BucketSize = &bucket
	}
	if !w.LastN.IsNull() && !w.LastN.IsUnknown() {
		lastN := sdktypes.BillingLastN(w.LastN.ValueInt64())
		resolved.LastN = &lastN
	}
	return resolved, diags
}

// configuredBillingRequest reads the request parameters a billing route takes,
// and only those: the window every route shares, plus the route's own resource
// filter when it has one. filterName is empty for the routes that do not.
// Decoding the whole state model out of configuration does not work for these:
// metadata is computed, so Terraform sends it null, and a computed nested
// object has no null to put in a plain struct field. Asking for the attributes
// a practitioner writes avoids the question, the way
// configuredEndpointId does for the per-endpoint listings.
func configuredBillingRequest(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse, filterName string) (billingWindow, types.String, bool) {
	var window billingWindow
	filter := types.StringNull()

	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("start_time"), &window.StartTime)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("end_time"), &window.EndTime)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("bucket_size"), &window.BucketSize)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("last_n"), &window.LastN)...)
	if filterName != "" {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(filterName), &filter)...)
	}
	if resp.Diagnostics.HasError() {
		return billingWindow{}, types.StringNull(), false
	}
	return window, filter, true
}

// billingWindowAttributes are the request parameters every billing route takes.
func billingWindowAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"start_time": schema.StringAttribute{
			Optional: true,
			Description: "Start of the billing period as an RFC 3339 timestamp. Defaults to 30 days ago. Snapped down to the start of its bucket, " +
				"so a boundary-aligned value (midnight for bucket_size \"day\") keeps the window from widening.",
		},
		"end_time": schema.StringAttribute{
			Optional: true,
			Description: "End of the billing period as an RFC 3339 timestamp, exclusive. Defaults to now. Snapped up to the end of the bucket it " +
				"lands in unless it is already on a boundary.",
		},
		"bucket_size": schema.StringAttribute{
			Optional:    true,
			Description: "Length of each time bucket: \"hour\", \"day\" (the API default), \"week\", \"month\", or \"year\".",
			Validators: []validator.String{
				stringvalidator.OneOf(
					string(sdktypes.BillingBucketSizeHour),
					string(sdktypes.BillingBucketSizeDay),
					string(sdktypes.BillingBucketSizeWeek),
					string(sdktypes.BillingBucketSizeMonth),
					string(sdktypes.BillingBucketSizeYear),
				),
			},
		},
		"last_n": schema.Int64Attribute{
			Optional: true,
			Description: "Return the last N buckets, ending with the current in-progress one: last_n 100 with bucket_size \"day\" is the last 100 days. " +
				"Names the window on its own: setting it together with start_time or end_time is rejected.",
			Validators: []validator.Int64{
				int64validator.AtLeast(1),
				int64validator.ConflictsWith(path.MatchRoot("start_time"), path.MatchRoot("end_time")),
			},
		},
	}
}

// billingQueryAttributes describes metadata.query: the window the API resolved
// the request to, plus the echo of the resource filter when the route has one.
func billingQueryAttributes(filterName, filterDescription string) map[string]schema.Attribute {
	attributes := map[string]schema.Attribute{
		"start_time":  schema.StringAttribute{Computed: true, Description: "Start of the resolved window, inclusive, in RFC 3339 format."},
		"end_time":    schema.StringAttribute{Computed: true, Description: "End of the resolved window, exclusive, in RFC 3339 format."},
		"bucket_size": schema.StringAttribute{Computed: true, Description: "Bucket length the records are grouped by."},
	}
	if filterName != "" {
		attributes[filterName] = schema.StringAttribute{Computed: true, Description: filterDescription}
	}
	return attributes
}

func billingAmount(description string) schema.Attribute {
	return schema.Float64Attribute{Computed: true, Description: description}
}

func billingBucketAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"start_time": schema.StringAttribute{Computed: true, Description: "Start of the bucket, inclusive, in RFC 3339 format."},
		"end_time":   schema.StringAttribute{Computed: true, Description: "End of the bucket, exclusive, in RFC 3339 format."},
	}
}

// withAttributes merges extra attributes into a base map; the caller owns both.
func withAttributes(base map[string]schema.Attribute, extra map[string]schema.Attribute) map[string]schema.Attribute {
	for name, attribute := range extra {
		base[name] = attribute
	}
	return base
}

type billingQueryModel struct {
	StartTime  types.String `tfsdk:"start_time"`
	EndTime    types.String `tfsdk:"end_time"`
	BucketSize types.String `tfsdk:"bucket_size"`
}

func flattenBillingQuery(query sdktypes.BillingQuery) billingQueryModel {
	return billingQueryModel{
		StartTime:  types.StringValue(query.StartTime.Format(time.RFC3339)),
		EndTime:    types.StringValue(query.EndTime.Format(time.RFC3339)),
		BucketSize: types.StringValue(string(query.BucketSize)),
	}
}

var (
	_ datasource.DataSource              = (*billingDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*billingDataSource)(nil)
)

func NewBillingDataSource() datasource.DataSource { return &billingDataSource{} }

type billingDataSource struct {
	client *runpod.Client
}

type aggregateBillingAmountsModel struct {
	PodGpuAmount                 types.Float64 `tfsdk:"pod_gpu_amount"`
	PodCpuAmount                 types.Float64 `tfsdk:"pod_cpu_amount"`
	PodDiskAmount                types.Float64 `tfsdk:"pod_disk_amount"`
	ServerlessGpuAmount          types.Float64 `tfsdk:"serverless_gpu_amount"`
	ServerlessCpuAmount          types.Float64 `tfsdk:"serverless_cpu_amount"`
	ServerlessDiskAmount         types.Float64 `tfsdk:"serverless_disk_amount"`
	ClusterGpuAmount             types.Float64 `tfsdk:"cluster_gpu_amount"`
	ClusterDiskAmount            types.Float64 `tfsdk:"cluster_disk_amount"`
	ClusterNetworkingAmount      types.Float64 `tfsdk:"cluster_networking_amount"`
	StorageStandardAmount        types.Float64 `tfsdk:"storage_standard_amount"`
	StorageHighPerformanceAmount types.Float64 `tfsdk:"storage_high_performance_amount"`
	EndpointAmount               types.Float64 `tfsdk:"endpoint_amount"`
	TotalAmount                  types.Float64 `tfsdk:"total_amount"`
}

type billingRecordModel struct {
	StartTime types.String                 `tfsdk:"start_time"`
	EndTime   types.String                 `tfsdk:"end_time"`
	Amounts   aggregateBillingAmountsModel `tfsdk:"amounts"`
}

type billingMetadataModel struct {
	Query       billingQueryModel            `tfsdk:"query"`
	RecordCount types.Int64                  `tfsdk:"record_count"`
	Totals      aggregateBillingAmountsModel `tfsdk:"totals"`
}

type billingDataSourceModel struct {
	StartTime  types.String         `tfsdk:"start_time"`
	EndTime    types.String         `tfsdk:"end_time"`
	BucketSize types.String         `tfsdk:"bucket_size"`
	LastN      types.Int64          `tfsdk:"last_n"`
	Metadata   billingMetadataModel `tfsdk:"metadata"`
	Records    []billingRecordModel `tfsdk:"records"`
}

// applyWindow echoes the configured window back into state, which is where the
// optional request parameters have to land now that state is built from the
// response rather than decoded out of configuration.
func (m *billingDataSourceModel) applyWindow(w billingWindow) {
	m.StartTime, m.EndTime, m.BucketSize, m.LastN = w.StartTime, w.EndTime, w.BucketSize, w.LastN
}

func aggregateBillingAmountAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"pod_gpu_amount":                  billingAmount("GPU pod compute cost in USD."),
		"pod_cpu_amount":                  billingAmount("CPU pod compute cost in USD."),
		"pod_disk_amount":                 billingAmount("Pod disk cost in USD."),
		"serverless_gpu_amount":           billingAmount("Serverless GPU compute cost in USD."),
		"serverless_cpu_amount":           billingAmount("Serverless CPU compute cost in USD."),
		"serverless_disk_amount":          billingAmount("Serverless disk cost in USD."),
		"cluster_gpu_amount":              billingAmount("Cluster GPU compute cost in USD."),
		"cluster_disk_amount":             billingAmount("Cluster disk cost in USD."),
		"cluster_networking_amount":       billingAmount("Cluster inter-node networking cost in USD."),
		"storage_standard_amount":         billingAmount("Standard network volume storage cost in USD."),
		"storage_high_performance_amount": billingAmount("High-performance network volume storage cost in USD."),
		"endpoint_amount":                 billingAmount("Runpod public endpoint cost in USD."),
		"total_amount":                    billingAmount("Total cost in USD across all resources."),
	}
}

func flattenAggregateBillingAmounts(amounts sdktypes.BillingAmounts) aggregateBillingAmountsModel {
	return aggregateBillingAmountsModel{
		PodGpuAmount:                 types.Float64Value(amounts.PodGpuAmount),
		PodCpuAmount:                 types.Float64Value(amounts.PodCpuAmount),
		PodDiskAmount:                types.Float64Value(amounts.PodDiskAmount),
		ServerlessGpuAmount:          types.Float64Value(amounts.ServerlessGpuAmount),
		ServerlessCpuAmount:          types.Float64Value(amounts.ServerlessCpuAmount),
		ServerlessDiskAmount:         types.Float64Value(amounts.ServerlessDiskAmount),
		ClusterGpuAmount:             types.Float64Value(amounts.ClusterGpuAmount),
		ClusterDiskAmount:            types.Float64Value(amounts.ClusterDiskAmount),
		ClusterNetworkingAmount:      types.Float64Value(amounts.ClusterNetworkingAmount),
		StorageStandardAmount:        types.Float64Value(amounts.StorageStandardAmount),
		StorageHighPerformanceAmount: types.Float64Value(amounts.StorageHighPerformanceAmount),
		EndpointAmount:               types.Float64Value(amounts.EndpointAmount),
		TotalAmount:                  types.Float64Value(amounts.TotalAmount),
	}
}

func (d *billingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing"
}

func (d *billingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Account-wide spend, time-bucketed, with every cost component broken out by resource. " + billingReplacementNote,
		Attributes: withAttributes(billingWindowAttributes(), map[string]schema.Attribute{
			"metadata": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The window the API resolved the request to, and the totals over it.",
				Attributes: map[string]schema.Attribute{
					"record_count": schema.Int64Attribute{Computed: true, Description: "Number of records returned."},
					"query": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Resolved query window and granularity.",
						Attributes:  billingQueryAttributes("", ""),
					},
					"totals": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Spend summed over every returned record.",
						Attributes:  aggregateBillingAmountAttributes(),
					},
				},
			},
			"records": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One record per time bucket.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: withAttributes(billingBucketAttributes(), map[string]schema.Attribute{
						"amounts": schema.SingleNestedAttribute{
							Computed:    true,
							Description: "Cost components of the bucket.",
							Attributes:  aggregateBillingAmountAttributes(),
						},
					}),
				},
			},
		}),
	}
}

func (d *billingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *billingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	configured, _, ok := configuredBillingRequest(ctx, req, resp, "")
	if !ok {
		return
	}

	window, diags := configured.resolve()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Billing.ListBillingWithResponse(ctx, &billing.ListBillingParams{
		StartTime:  window.StartTime,
		EndTime:    window.EndTime,
		BucketSize: window.BucketSize,
		LastN:      window.LastN,
	})
	if err != nil {
		resp.Diagnostics.AddError("Reading billing failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Reading billing failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	var state billingDataSourceModel
	state.applyWindow(configured)
	flattenBilling(*apiResp.JSON200, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenBilling(response sdktypes.ListBillingResponse, state *billingDataSourceModel) {
	state.Metadata = billingMetadataModel{
		Query:       flattenBillingQuery(response.Metadata.Query),
		RecordCount: types.Int64Value(int64(response.Metadata.RecordCount)),
		Totals:      flattenAggregateBillingAmounts(response.Metadata.Totals),
	}
	state.Records = []billingRecordModel{}
	for _, record := range response.Records {
		state.Records = append(state.Records, billingRecordModel{
			StartTime: types.StringValue(record.StartTime.Format(time.RFC3339)),
			EndTime:   types.StringValue(record.EndTime.Format(time.RFC3339)),
			Amounts: flattenAggregateBillingAmounts(sdktypes.BillingAmounts{
				PodGpuAmount:                 record.PodGpuAmount,
				PodCpuAmount:                 record.PodCpuAmount,
				PodDiskAmount:                record.PodDiskAmount,
				ServerlessGpuAmount:          record.ServerlessGpuAmount,
				ServerlessCpuAmount:          record.ServerlessCpuAmount,
				ServerlessDiskAmount:         record.ServerlessDiskAmount,
				ClusterGpuAmount:             record.ClusterGpuAmount,
				ClusterDiskAmount:            record.ClusterDiskAmount,
				ClusterNetworkingAmount:      record.ClusterNetworkingAmount,
				StorageStandardAmount:        record.StorageStandardAmount,
				StorageHighPerformanceAmount: record.StorageHighPerformanceAmount,
				EndpointAmount:               record.EndpointAmount,
				TotalAmount:                  record.TotalAmount,
			}),
		})
	}
}

var (
	_ datasource.DataSource              = (*podBillingDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*podBillingDataSource)(nil)
)

func NewPodBillingDataSource() datasource.DataSource { return &podBillingDataSource{} }

type podBillingDataSource struct {
	client *runpod.Client
}

type podBillingAmountsModel struct {
	GpuAmount   types.Float64 `tfsdk:"gpu_amount"`
	CpuAmount   types.Float64 `tfsdk:"cpu_amount"`
	DiskAmount  types.Float64 `tfsdk:"disk_amount"`
	TotalAmount types.Float64 `tfsdk:"total_amount"`
}

type podBillingQueryModel struct {
	StartTime  types.String `tfsdk:"start_time"`
	EndTime    types.String `tfsdk:"end_time"`
	BucketSize types.String `tfsdk:"bucket_size"`
	PodId      types.String `tfsdk:"pod_id"`
}

type podBillingMetadataModel struct {
	Query          podBillingQueryModel   `tfsdk:"query"`
	RecordCount    types.Int64            `tfsdk:"record_count"`
	UniquePodCount types.Int64            `tfsdk:"unique_pod_count"`
	Totals         podBillingAmountsModel `tfsdk:"totals"`
}

type podBillingRecordModel struct {
	PodId     types.String           `tfsdk:"pod_id"`
	StartTime types.String           `tfsdk:"start_time"`
	EndTime   types.String           `tfsdk:"end_time"`
	Amounts   podBillingAmountsModel `tfsdk:"amounts"`
}

type podBillingDataSourceModel struct {
	StartTime  types.String            `tfsdk:"start_time"`
	EndTime    types.String            `tfsdk:"end_time"`
	BucketSize types.String            `tfsdk:"bucket_size"`
	LastN      types.Int64             `tfsdk:"last_n"`
	PodId      types.String            `tfsdk:"pod_id"`
	Metadata   podBillingMetadataModel `tfsdk:"metadata"`
	Records    []podBillingRecordModel `tfsdk:"records"`
}

func (m *podBillingDataSourceModel) applyWindow(w billingWindow) {
	m.StartTime, m.EndTime, m.BucketSize, m.LastN = w.StartTime, w.EndTime, w.BucketSize, w.LastN
}

func podBillingAmountAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"gpu_amount":   billingAmount("GPU pod compute cost in USD."),
		"cpu_amount":   billingAmount("CPU pod compute cost in USD."),
		"disk_amount":  billingAmount("Pod disk cost in USD."),
		"total_amount": billingAmount("Total pod cost in USD."),
	}
}

func flattenPodBillingAmounts(amounts sdktypes.PodBillingAmounts) podBillingAmountsModel {
	return podBillingAmountsModel{
		GpuAmount:   types.Float64Value(amounts.GpuAmount),
		CpuAmount:   types.Float64Value(amounts.CpuAmount),
		DiskAmount:  types.Float64Value(amounts.DiskAmount),
		TotalAmount: types.Float64Value(amounts.TotalAmount),
	}
}

func (d *podBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pod_billing"
}

func (d *podBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Pod spend, time-bucketed, covering both GPU and CPU pods. Without pod_id the API emits one record per pod per bucket. " + billingReplacementNote,
		Attributes: withAttributes(billingWindowAttributes(), map[string]schema.Attribute{
			"pod_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one pod."},
			"metadata": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The window the API resolved the request to, and the totals over it.",
				Attributes: map[string]schema.Attribute{
					"record_count":     schema.Int64Attribute{Computed: true, Description: "Number of records returned: buckets times distinct pods."},
					"unique_pod_count": schema.Int64Attribute{Computed: true, Description: "Number of distinct pods the records span."},
					"query": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Resolved query window, granularity, and filter.",
						Attributes:  billingQueryAttributes("pod_id", "The pod_id filter the API applied; null when unfiltered."),
					},
					"totals": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Spend summed over every returned record.",
						Attributes:  podBillingAmountAttributes(),
					},
				},
			},
			"records": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One record per pod per time bucket.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: withAttributes(billingBucketAttributes(), map[string]schema.Attribute{
						"pod_id": schema.StringAttribute{Computed: true, Description: "The pod this record bills."},
						"amounts": schema.SingleNestedAttribute{
							Computed:    true,
							Description: "Cost components of the bucket.",
							Attributes:  podBillingAmountAttributes(),
						},
					}),
				},
			},
		}),
	}
}

func (d *podBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *podBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	configured, podId, ok := configuredBillingRequest(ctx, req, resp, "pod_id")
	if !ok {
		return
	}

	window, diags := configured.resolve()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Billing.ListPodBillingWithResponse(ctx, &billing.ListPodBillingParams{
		StartTime:  window.StartTime,
		EndTime:    window.EndTime,
		BucketSize: window.BucketSize,
		LastN:      window.LastN,
		PodId:      podId.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Reading pod billing failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Reading pod billing failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	var state podBillingDataSourceModel
	state.applyWindow(configured)
	state.PodId = podId
	flattenPodBilling(*apiResp.JSON200, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenPodBilling(response sdktypes.ListPodBillingResponse, state *podBillingDataSourceModel) {
	state.Metadata = podBillingMetadataModel{
		Query: podBillingQueryModel{
			StartTime:  types.StringValue(response.Metadata.Query.StartTime.Format(time.RFC3339)),
			EndTime:    types.StringValue(response.Metadata.Query.EndTime.Format(time.RFC3339)),
			BucketSize: types.StringValue(string(response.Metadata.Query.BucketSize)),
			PodId:      types.StringPointerValue(response.Metadata.Query.PodId),
		},
		RecordCount:    types.Int64Value(int64(response.Metadata.RecordCount)),
		UniquePodCount: types.Int64Value(int64(response.Metadata.UniquePodCount)),
		Totals:         flattenPodBillingAmounts(response.Metadata.Totals),
	}
	state.Records = []podBillingRecordModel{}
	for _, record := range response.Records {
		state.Records = append(state.Records, podBillingRecordModel{
			PodId:     types.StringValue(record.PodId),
			StartTime: types.StringValue(record.StartTime.Format(time.RFC3339)),
			EndTime:   types.StringValue(record.EndTime.Format(time.RFC3339)),
			Amounts: flattenPodBillingAmounts(sdktypes.PodBillingAmounts{
				GpuAmount:   record.GpuAmount,
				CpuAmount:   record.CpuAmount,
				DiskAmount:  record.DiskAmount,
				TotalAmount: record.TotalAmount,
			}),
		})
	}
}

var (
	_ datasource.DataSource              = (*serverlessBillingDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*serverlessBillingDataSource)(nil)
)

func NewServerlessBillingDataSource() datasource.DataSource { return &serverlessBillingDataSource{} }

type serverlessBillingDataSource struct {
	client *runpod.Client
}

type serverlessBillingAmountsModel struct {
	GpuAmount   types.Float64 `tfsdk:"gpu_amount"`
	CpuAmount   types.Float64 `tfsdk:"cpu_amount"`
	DiskAmount  types.Float64 `tfsdk:"disk_amount"`
	TotalAmount types.Float64 `tfsdk:"total_amount"`
}

type serverlessBillingQueryModel struct {
	StartTime    types.String `tfsdk:"start_time"`
	EndTime      types.String `tfsdk:"end_time"`
	BucketSize   types.String `tfsdk:"bucket_size"`
	ServerlessId types.String `tfsdk:"serverless_id"`
}

type serverlessBillingMetadataModel struct {
	Query                 serverlessBillingQueryModel   `tfsdk:"query"`
	RecordCount           types.Int64                   `tfsdk:"record_count"`
	UniqueServerlessCount types.Int64                   `tfsdk:"unique_serverless_count"`
	Totals                serverlessBillingAmountsModel `tfsdk:"totals"`
}

type serverlessBillingRecordModel struct {
	ServerlessId types.String                  `tfsdk:"serverless_id"`
	StartTime    types.String                  `tfsdk:"start_time"`
	EndTime      types.String                  `tfsdk:"end_time"`
	Amounts      serverlessBillingAmountsModel `tfsdk:"amounts"`
}

type serverlessBillingDataSourceModel struct {
	StartTime    types.String                   `tfsdk:"start_time"`
	EndTime      types.String                   `tfsdk:"end_time"`
	BucketSize   types.String                   `tfsdk:"bucket_size"`
	LastN        types.Int64                    `tfsdk:"last_n"`
	ServerlessId types.String                   `tfsdk:"serverless_id"`
	Metadata     serverlessBillingMetadataModel `tfsdk:"metadata"`
	Records      []serverlessBillingRecordModel `tfsdk:"records"`
}

func (m *serverlessBillingDataSourceModel) applyWindow(w billingWindow) {
	m.StartTime, m.EndTime, m.BucketSize, m.LastN = w.StartTime, w.EndTime, w.BucketSize, w.LastN
}

func serverlessBillingAmountAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"gpu_amount":   billingAmount("Serverless GPU compute cost in USD."),
		"cpu_amount":   billingAmount("Serverless CPU compute cost in USD."),
		"disk_amount":  billingAmount("Serverless disk cost in USD."),
		"total_amount": billingAmount("Total serverless cost in USD."),
	}
}

func flattenServerlessBillingAmounts(amounts sdktypes.ServerlessBillingAmounts) serverlessBillingAmountsModel {
	return serverlessBillingAmountsModel{
		GpuAmount:   types.Float64Value(amounts.GpuAmount),
		CpuAmount:   types.Float64Value(amounts.CpuAmount),
		DiskAmount:  types.Float64Value(amounts.DiskAmount),
		TotalAmount: types.Float64Value(amounts.TotalAmount),
	}
}

func (d *serverlessBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_serverless_billing"
}

func (d *serverlessBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Serverless spend, time-bucketed. Without serverless_id the API emits one record per endpoint per bucket. " + billingReplacementNote,
		Attributes: withAttributes(billingWindowAttributes(), map[string]schema.Attribute{
			"serverless_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one Serverless endpoint."},
			"metadata": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The window the API resolved the request to, and the totals over it.",
				Attributes: map[string]schema.Attribute{
					"record_count":            schema.Int64Attribute{Computed: true, Description: "Number of records returned: buckets times distinct endpoints."},
					"unique_serverless_count": schema.Int64Attribute{Computed: true, Description: "Number of distinct Serverless endpoints the records span."},
					"query": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Resolved query window, granularity, and filter.",
						Attributes:  billingQueryAttributes("serverless_id", "The serverless_id filter the API applied; null when unfiltered."),
					},
					"totals": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Spend summed over every returned record.",
						Attributes:  serverlessBillingAmountAttributes(),
					},
				},
			},
			"records": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One record per Serverless endpoint per time bucket.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: withAttributes(billingBucketAttributes(), map[string]schema.Attribute{
						"serverless_id": schema.StringAttribute{Computed: true, Description: "The Serverless endpoint this record bills."},
						"amounts": schema.SingleNestedAttribute{
							Computed:    true,
							Description: "Cost components of the bucket.",
							Attributes:  serverlessBillingAmountAttributes(),
						},
					}),
				},
			},
		}),
	}
}

func (d *serverlessBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *serverlessBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	configured, serverlessId, ok := configuredBillingRequest(ctx, req, resp, "serverless_id")
	if !ok {
		return
	}

	window, diags := configured.resolve()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Billing.ListServerlessBillingWithResponse(ctx, &billing.ListServerlessBillingParams{
		StartTime:    window.StartTime,
		EndTime:      window.EndTime,
		BucketSize:   window.BucketSize,
		LastN:        window.LastN,
		ServerlessId: serverlessId.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Reading serverless billing failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Reading serverless billing failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	var state serverlessBillingDataSourceModel
	state.applyWindow(configured)
	state.ServerlessId = serverlessId
	flattenServerlessBilling(*apiResp.JSON200, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenServerlessBilling(response sdktypes.ListServerlessBillingResponse, state *serverlessBillingDataSourceModel) {
	state.Metadata = serverlessBillingMetadataModel{
		Query: serverlessBillingQueryModel{
			StartTime:    types.StringValue(response.Metadata.Query.StartTime.Format(time.RFC3339)),
			EndTime:      types.StringValue(response.Metadata.Query.EndTime.Format(time.RFC3339)),
			BucketSize:   types.StringValue(string(response.Metadata.Query.BucketSize)),
			ServerlessId: types.StringPointerValue(response.Metadata.Query.ServerlessId),
		},
		RecordCount:           types.Int64Value(int64(response.Metadata.RecordCount)),
		UniqueServerlessCount: types.Int64Value(int64(response.Metadata.UniqueServerlessCount)),
		Totals:                flattenServerlessBillingAmounts(response.Metadata.Totals),
	}
	state.Records = []serverlessBillingRecordModel{}
	for _, record := range response.Records {
		state.Records = append(state.Records, serverlessBillingRecordModel{
			ServerlessId: types.StringValue(record.ServerlessId),
			StartTime:    types.StringValue(record.StartTime.Format(time.RFC3339)),
			EndTime:      types.StringValue(record.EndTime.Format(time.RFC3339)),
			Amounts: flattenServerlessBillingAmounts(sdktypes.ServerlessBillingAmounts{
				GpuAmount:   record.GpuAmount,
				CpuAmount:   record.CpuAmount,
				DiskAmount:  record.DiskAmount,
				TotalAmount: record.TotalAmount,
			}),
		})
	}
}

var (
	_ datasource.DataSource              = (*endpointBillingDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*endpointBillingDataSource)(nil)
)

func NewEndpointBillingDataSource() datasource.DataSource { return &endpointBillingDataSource{} }

type endpointBillingDataSource struct {
	client *runpod.Client
}

type endpointBillingAmountsModel struct {
	TotalAmount types.Float64 `tfsdk:"total_amount"`
}

type endpointBillingMetadataModel struct {
	Query       billingQueryModel           `tfsdk:"query"`
	RecordCount types.Int64                 `tfsdk:"record_count"`
	Totals      endpointBillingAmountsModel `tfsdk:"totals"`
}

type endpointBillingRecordModel struct {
	StartTime types.String                `tfsdk:"start_time"`
	EndTime   types.String                `tfsdk:"end_time"`
	Amounts   endpointBillingAmountsModel `tfsdk:"amounts"`
}

type endpointBillingDataSourceModel struct {
	StartTime  types.String                 `tfsdk:"start_time"`
	EndTime    types.String                 `tfsdk:"end_time"`
	BucketSize types.String                 `tfsdk:"bucket_size"`
	LastN      types.Int64                  `tfsdk:"last_n"`
	Metadata   endpointBillingMetadataModel `tfsdk:"metadata"`
	Records    []endpointBillingRecordModel `tfsdk:"records"`
}

func (m *endpointBillingDataSourceModel) applyWindow(w billingWindow) {
	m.StartTime, m.EndTime, m.BucketSize, m.LastN = w.StartTime, w.EndTime, w.BucketSize, w.LastN
}

func endpointBillingAmountAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"total_amount": billingAmount("Total public endpoint cost in USD."),
	}
}

func (d *endpointBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_billing"
}

func (d *endpointBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Spend on Runpod's own public endpoints, time-bucketed. This route takes no resource filter. " + billingReplacementNote,
		Attributes: withAttributes(billingWindowAttributes(), map[string]schema.Attribute{
			"metadata": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The window the API resolved the request to, and the totals over it.",
				Attributes: map[string]schema.Attribute{
					"record_count": schema.Int64Attribute{Computed: true, Description: "Number of records returned."},
					"query": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Resolved query window and granularity.",
						Attributes:  billingQueryAttributes("", ""),
					},
					"totals": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Spend summed over every returned record.",
						Attributes:  endpointBillingAmountAttributes(),
					},
				},
			},
			"records": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One record per time bucket.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: withAttributes(billingBucketAttributes(), map[string]schema.Attribute{
						"amounts": schema.SingleNestedAttribute{
							Computed:    true,
							Description: "Cost components of the bucket.",
							Attributes:  endpointBillingAmountAttributes(),
						},
					}),
				},
			},
		}),
	}
}

func (d *endpointBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *endpointBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	configured, _, ok := configuredBillingRequest(ctx, req, resp, "")
	if !ok {
		return
	}

	window, diags := configured.resolve()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Billing.ListEndpointBillingWithResponse(ctx, &billing.ListEndpointBillingParams{
		StartTime:  window.StartTime,
		EndTime:    window.EndTime,
		BucketSize: window.BucketSize,
		LastN:      window.LastN,
	})
	if err != nil {
		resp.Diagnostics.AddError("Reading endpoint billing failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Reading endpoint billing failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	var state endpointBillingDataSourceModel
	state.applyWindow(configured)
	flattenEndpointBilling(*apiResp.JSON200, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenEndpointBilling(response sdktypes.ListEndpointBillingResponse, state *endpointBillingDataSourceModel) {
	state.Metadata = endpointBillingMetadataModel{
		Query:       flattenBillingQuery(response.Metadata.Query),
		RecordCount: types.Int64Value(int64(response.Metadata.RecordCount)),
		Totals:      endpointBillingAmountsModel{TotalAmount: types.Float64Value(response.Metadata.Totals.TotalAmount)},
	}
	state.Records = []endpointBillingRecordModel{}
	for _, record := range response.Records {
		state.Records = append(state.Records, endpointBillingRecordModel{
			StartTime: types.StringValue(record.StartTime.Format(time.RFC3339)),
			EndTime:   types.StringValue(record.EndTime.Format(time.RFC3339)),
			Amounts:   endpointBillingAmountsModel{TotalAmount: types.Float64Value(record.TotalAmount)},
		})
	}
}

var (
	_ datasource.DataSource              = (*networkVolumeBillingDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*networkVolumeBillingDataSource)(nil)
)

func NewNetworkVolumeBillingDataSource() datasource.DataSource {
	return &networkVolumeBillingDataSource{}
}

type networkVolumeBillingDataSource struct {
	client *runpod.Client
}

type networkVolumeBillingAmountsModel struct {
	StandardAmount        types.Float64 `tfsdk:"standard_amount"`
	HighPerformanceAmount types.Float64 `tfsdk:"high_performance_amount"`
	TotalAmount           types.Float64 `tfsdk:"total_amount"`
}

type networkVolumeBillingQueryModel struct {
	StartTime       types.String `tfsdk:"start_time"`
	EndTime         types.String `tfsdk:"end_time"`
	BucketSize      types.String `tfsdk:"bucket_size"`
	NetworkVolumeId types.String `tfsdk:"network_volume_id"`
}

type networkVolumeBillingMetadataModel struct {
	Query                    networkVolumeBillingQueryModel   `tfsdk:"query"`
	RecordCount              types.Int64                      `tfsdk:"record_count"`
	UniqueNetworkVolumeCount types.Int64                      `tfsdk:"unique_network_volume_count"`
	Totals                   networkVolumeBillingAmountsModel `tfsdk:"totals"`
}

type networkVolumeBillingRecordModel struct {
	NetworkVolumeId types.String                     `tfsdk:"network_volume_id"`
	StartTime       types.String                     `tfsdk:"start_time"`
	EndTime         types.String                     `tfsdk:"end_time"`
	Amounts         networkVolumeBillingAmountsModel `tfsdk:"amounts"`
}

type networkVolumeBillingDataSourceModel struct {
	StartTime       types.String                      `tfsdk:"start_time"`
	EndTime         types.String                      `tfsdk:"end_time"`
	BucketSize      types.String                      `tfsdk:"bucket_size"`
	LastN           types.Int64                       `tfsdk:"last_n"`
	NetworkVolumeId types.String                      `tfsdk:"network_volume_id"`
	Metadata        networkVolumeBillingMetadataModel `tfsdk:"metadata"`
	Records         []networkVolumeBillingRecordModel `tfsdk:"records"`
}

func (m *networkVolumeBillingDataSourceModel) applyWindow(w billingWindow) {
	m.StartTime, m.EndTime, m.BucketSize, m.LastN = w.StartTime, w.EndTime, w.BucketSize, w.LastN
}

func networkVolumeBillingAmountAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"standard_amount":         billingAmount("Standard storage cost in USD."),
		"high_performance_amount": billingAmount("High-performance storage cost in USD."),
		"total_amount":            billingAmount("Total network volume cost in USD across both tiers."),
	}
}

func flattenNetworkVolumeBillingAmounts(amounts sdktypes.NetworkVolumeBillingAmounts) networkVolumeBillingAmountsModel {
	return networkVolumeBillingAmountsModel{
		StandardAmount:        types.Float64Value(amounts.StandardAmount),
		HighPerformanceAmount: types.Float64Value(amounts.HighPerformanceAmount),
		TotalAmount:           types.Float64Value(amounts.TotalAmount),
	}
}

func (d *networkVolumeBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_volume_billing"
}

func (d *networkVolumeBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Network volume storage spend, time-bucketed and split by storage tier. Without network_volume_id the API emits one record per volume per bucket. " + billingReplacementNote,
		Attributes: withAttributes(billingWindowAttributes(), map[string]schema.Attribute{
			"network_volume_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one network volume."},
			"metadata": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The window the API resolved the request to, and the totals over it.",
				Attributes: map[string]schema.Attribute{
					"record_count":                schema.Int64Attribute{Computed: true, Description: "Number of records returned: buckets times distinct volumes."},
					"unique_network_volume_count": schema.Int64Attribute{Computed: true, Description: "Number of distinct network volumes the records span."},
					"query": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Resolved query window, granularity, and filter.",
						Attributes:  billingQueryAttributes("network_volume_id", "The network_volume_id filter the API applied; null when unfiltered."),
					},
					"totals": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Spend summed over every returned record.",
						Attributes:  networkVolumeBillingAmountAttributes(),
					},
				},
			},
			"records": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One record per network volume per time bucket.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: withAttributes(billingBucketAttributes(), map[string]schema.Attribute{
						"network_volume_id": schema.StringAttribute{Computed: true, Description: "The network volume this record bills."},
						"amounts": schema.SingleNestedAttribute{
							Computed:    true,
							Description: "Cost components of the bucket.",
							Attributes:  networkVolumeBillingAmountAttributes(),
						},
					}),
				},
			},
		}),
	}
}

func (d *networkVolumeBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *networkVolumeBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	configured, networkVolumeId, ok := configuredBillingRequest(ctx, req, resp, "network_volume_id")
	if !ok {
		return
	}

	window, diags := configured.resolve()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Billing.ListNetworkVolumeBillingWithResponse(ctx, &billing.ListNetworkVolumeBillingParams{
		StartTime:       window.StartTime,
		EndTime:         window.EndTime,
		BucketSize:      window.BucketSize,
		LastN:           window.LastN,
		NetworkVolumeId: networkVolumeId.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Reading network volume billing failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Reading network volume billing failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	var state networkVolumeBillingDataSourceModel
	state.applyWindow(configured)
	state.NetworkVolumeId = networkVolumeId
	flattenNetworkVolumeBilling(*apiResp.JSON200, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenNetworkVolumeBilling(response sdktypes.ListNetworkVolumeBillingResponse, state *networkVolumeBillingDataSourceModel) {
	state.Metadata = networkVolumeBillingMetadataModel{
		Query: networkVolumeBillingQueryModel{
			StartTime:       types.StringValue(response.Metadata.Query.StartTime.Format(time.RFC3339)),
			EndTime:         types.StringValue(response.Metadata.Query.EndTime.Format(time.RFC3339)),
			BucketSize:      types.StringValue(string(response.Metadata.Query.BucketSize)),
			NetworkVolumeId: types.StringPointerValue(response.Metadata.Query.NetworkVolumeId),
		},
		RecordCount:              types.Int64Value(int64(response.Metadata.RecordCount)),
		UniqueNetworkVolumeCount: types.Int64Value(int64(response.Metadata.UniqueNetworkVolumeCount)),
		Totals:                   flattenNetworkVolumeBillingAmounts(response.Metadata.Totals),
	}
	state.Records = []networkVolumeBillingRecordModel{}
	for _, record := range response.Records {
		state.Records = append(state.Records, networkVolumeBillingRecordModel{
			NetworkVolumeId: types.StringValue(record.NetworkVolumeId),
			StartTime:       types.StringValue(record.StartTime.Format(time.RFC3339)),
			EndTime:         types.StringValue(record.EndTime.Format(time.RFC3339)),
			Amounts: flattenNetworkVolumeBillingAmounts(sdktypes.NetworkVolumeBillingAmounts{
				StandardAmount:        record.StandardAmount,
				HighPerformanceAmount: record.HighPerformanceAmount,
				TotalAmount:           record.TotalAmount,
			}),
		})
	}
}

var (
	_ datasource.DataSource              = (*clusterBillingDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterBillingDataSource)(nil)
)

func NewClusterBillingDataSource() datasource.DataSource { return &clusterBillingDataSource{} }

type clusterBillingDataSource struct {
	client *runpod.Client
}

type clusterBillingAmountsModel struct {
	GpuAmount        types.Float64 `tfsdk:"gpu_amount"`
	DiskAmount       types.Float64 `tfsdk:"disk_amount"`
	NetworkingAmount types.Float64 `tfsdk:"networking_amount"`
	TotalAmount      types.Float64 `tfsdk:"total_amount"`
}

type clusterBillingQueryModel struct {
	StartTime  types.String `tfsdk:"start_time"`
	EndTime    types.String `tfsdk:"end_time"`
	BucketSize types.String `tfsdk:"bucket_size"`
	ClusterId  types.String `tfsdk:"cluster_id"`
}

type clusterBillingMetadataModel struct {
	Query              clusterBillingQueryModel   `tfsdk:"query"`
	RecordCount        types.Int64                `tfsdk:"record_count"`
	UniqueClusterCount types.Int64                `tfsdk:"unique_cluster_count"`
	Totals             clusterBillingAmountsModel `tfsdk:"totals"`
}

type clusterBillingRecordModel struct {
	ClusterId types.String               `tfsdk:"cluster_id"`
	StartTime types.String               `tfsdk:"start_time"`
	EndTime   types.String               `tfsdk:"end_time"`
	Amounts   clusterBillingAmountsModel `tfsdk:"amounts"`
}

type clusterBillingDataSourceModel struct {
	StartTime  types.String                `tfsdk:"start_time"`
	EndTime    types.String                `tfsdk:"end_time"`
	BucketSize types.String                `tfsdk:"bucket_size"`
	LastN      types.Int64                 `tfsdk:"last_n"`
	ClusterId  types.String                `tfsdk:"cluster_id"`
	Metadata   clusterBillingMetadataModel `tfsdk:"metadata"`
	Records    []clusterBillingRecordModel `tfsdk:"records"`
}

func (m *clusterBillingDataSourceModel) applyWindow(w billingWindow) {
	m.StartTime, m.EndTime, m.BucketSize, m.LastN = w.StartTime, w.EndTime, w.BucketSize, w.LastN
}

func clusterBillingAmountAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"gpu_amount":        billingAmount("Cluster GPU compute cost in USD."),
		"disk_amount":       billingAmount("Cluster disk cost in USD."),
		"networking_amount": billingAmount("Cluster inter-node networking cost in USD."),
		"total_amount":      billingAmount("Total Instant Cluster cost in USD."),
	}
}

func flattenClusterBillingAmounts(amounts sdktypes.ClusterBillingAmounts) clusterBillingAmountsModel {
	return clusterBillingAmountsModel{
		GpuAmount:        types.Float64Value(amounts.GpuAmount),
		DiskAmount:       types.Float64Value(amounts.DiskAmount),
		NetworkingAmount: types.Float64Value(amounts.NetworkingAmount),
		TotalAmount:      types.Float64Value(amounts.TotalAmount),
	}
}

func (d *clusterBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_billing"
}

func (d *clusterBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Instant Cluster spend, time-bucketed. Clusters are GPU-only, so there is no CPU component. Without cluster_id the API emits one record per cluster per bucket. " + billingReplacementNote,
		Attributes: withAttributes(billingWindowAttributes(), map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one cluster."},
			"metadata": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The window the API resolved the request to, and the totals over it.",
				Attributes: map[string]schema.Attribute{
					"record_count":         schema.Int64Attribute{Computed: true, Description: "Number of records returned: buckets times distinct clusters."},
					"unique_cluster_count": schema.Int64Attribute{Computed: true, Description: "Number of distinct clusters the records span."},
					"query": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Resolved query window, granularity, and filter.",
						Attributes:  billingQueryAttributes("cluster_id", "The cluster_id filter the API applied; null when unfiltered."),
					},
					"totals": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Spend summed over every returned record.",
						Attributes:  clusterBillingAmountAttributes(),
					},
				},
			},
			"records": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One record per cluster per time bucket.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: withAttributes(billingBucketAttributes(), map[string]schema.Attribute{
						"cluster_id": schema.StringAttribute{Computed: true, Description: "The cluster this record bills."},
						"amounts": schema.SingleNestedAttribute{
							Computed:    true,
							Description: "Cost components of the bucket.",
							Attributes:  clusterBillingAmountAttributes(),
						},
					}),
				},
			},
		}),
	}
}

func (d *clusterBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *clusterBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	configured, clusterId, ok := configuredBillingRequest(ctx, req, resp, "cluster_id")
	if !ok {
		return
	}

	window, diags := configured.resolve()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Billing.ListClusterBillingWithResponse(ctx, &billing.ListClusterBillingParams{
		StartTime:  window.StartTime,
		EndTime:    window.EndTime,
		BucketSize: window.BucketSize,
		LastN:      window.LastN,
		ClusterId:  clusterId.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Reading cluster billing failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Reading cluster billing failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	var state clusterBillingDataSourceModel
	state.applyWindow(configured)
	state.ClusterId = clusterId
	flattenClusterBilling(*apiResp.JSON200, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenClusterBilling(response sdktypes.ListClusterBillingResponse, state *clusterBillingDataSourceModel) {
	state.Metadata = clusterBillingMetadataModel{
		Query: clusterBillingQueryModel{
			StartTime:  types.StringValue(response.Metadata.Query.StartTime.Format(time.RFC3339)),
			EndTime:    types.StringValue(response.Metadata.Query.EndTime.Format(time.RFC3339)),
			BucketSize: types.StringValue(string(response.Metadata.Query.BucketSize)),
			ClusterId:  types.StringPointerValue(response.Metadata.Query.ClusterId),
		},
		RecordCount:        types.Int64Value(int64(response.Metadata.RecordCount)),
		UniqueClusterCount: types.Int64Value(int64(response.Metadata.UniqueClusterCount)),
		Totals:             flattenClusterBillingAmounts(response.Metadata.Totals),
	}
	state.Records = []clusterBillingRecordModel{}
	for _, record := range response.Records {
		state.Records = append(state.Records, clusterBillingRecordModel{
			ClusterId: types.StringValue(record.ClusterId),
			StartTime: types.StringValue(record.StartTime.Format(time.RFC3339)),
			EndTime:   types.StringValue(record.EndTime.Format(time.RFC3339)),
			Amounts: flattenClusterBillingAmounts(sdktypes.ClusterBillingAmounts{
				GpuAmount:        record.GpuAmount,
				DiskAmount:       record.DiskAmount,
				NetworkingAmount: record.NetworkingAmount,
				TotalAmount:      record.TotalAmount,
			}),
		})
	}
}
