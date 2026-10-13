package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// The three deprecated billing data sources the 1.x provider shipped:
// runpod_billing_pod, runpod_billing_network_volume and
// runpod_billing_endpoint. They exist so a 1.0.8 configuration keeps planning
// unchanged; every attribute name, filter and record field is the one that
// release declared. The records come from the REST v1 billing routes, which
// carry per-record fields the REST v2 billing surface does not report yet:
// gpuTypeId, diskSpaceBilledGb, endpointId and timeBilledMs.
//
// New configurations should use runpod_pod_billing,
// runpod_network_volume_billing and runpod_serverless_billing instead:
// REST v2, time-bucketed, with the cost components broken out. Serverless
// endpoint spend is runpod_serverless_billing; the similarly named
// runpod_endpoint_billing reports spend on Runpod's own public endpoints.
//
// Two 1.0.8 behaviours are deliberately not reproduced: amount and time were
// read with unchecked type assertions that panicked when a record omitted
// either (they read null here), and end_time was dropped from the state of the
// network volume and endpoint data sources, which Terraform rejects as an
// inconsistent result.

const legacyBillingDeprecation = "REST v1; use runpod_pod_billing / runpod_network_volume_billing / " +
	"runpod_serverless_billing (REST v2); removed when REST v2 billing carries these fields"

// legacyBillingWindowAttributes is the query window 1.0.8 accepted: a bucket
// size and an end time. It never sent a start time, and the REST v1 routes
// take none, so none is declared here.
func legacyBillingWindowAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"bucket_size": schema.StringAttribute{
			Optional:    true,
			Description: "Length of each time bucket: hour, day, week, month, or year. Defaults to day.",
		},
		"end_time": schema.StringAttribute{
			Optional:    true,
			Description: "End of the reported window as an ISO datetime. Defaults to the current time.",
		},
	}
}

// legacyBillingRecordAttributes is the part of a record every REST v1 billing
// route carries.
func legacyBillingRecordAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"amount": schema.Float64Attribute{Computed: true, Description: "Amount charged in USD."},
		"time":   schema.StringAttribute{Computed: true, Description: "Start of the bucket this record bills, as an ISO datetime."},
	}
}

// legacyBillingRecords wraps a route's record list in the shared list
// attribute, under the 1.0.8 name.
func legacyBillingRecords(attributes map[string]schema.Attribute) schema.Attribute {
	return schema.ListNestedAttribute{
		Computed:     true,
		Description:  "One record per time bucket, as the REST v1 billing route returns them.",
		NestedObject: schema.NestedAttributeObject{Attributes: withAttributes(legacyBillingRecordAttributes(), attributes)},
	}
}

// appendLegacyBillingFailure records a REST v1 billing read that did not
// answer. Every failure here is an error, filtered or not, and a 404 included:
// the warn-and-null rule the other REST v1 shims follow is for an attribute
// hanging off a resource the v2 API still describes, and these data sources
// have no source but the legacy host. The records are the whole body.
// Answering with null records hands the first `billing_records[0]` an "Attempt
// to index null value" and leaves the real cause in a warning that has already
// scrolled past.
func appendLegacyBillingFailure(diags *diag.Diagnostics, route string, err error) {
	diags.AddError(
		fmt.Sprintf("Reading the deprecated REST v1 %s failed", route),
		fmt.Sprintf(
			"%s\n\nThis deprecated data source reads the REST v1 billing route and nothing else. The REST v2 billing data sources do not depend on that host.",
			err.Error(),
		),
	)
}

// appendMissingLegacyClient reports a read that cannot happen at all because
// the provider handed over no REST v1 client. These data sources have no other
// source for their records, so returning quietly would answer the read with a
// null object and the message "Provider produced a null object", which says
// nothing about what to do.
func appendMissingLegacyClient(diags *diag.Diagnostics, route string) {
	diags.AddError(
		fmt.Sprintf("Reading the deprecated REST v1 %s failed", route),
		"The provider has no REST v1 client configured, and this deprecated data source reads that host only. "+
			"Configure the provider (rest_v1_url, or the RUNPOD_REST_V1_URL environment variable) or move to the "+
			"REST v2 billing data sources.",
	)
}

var (
	_ datasource.DataSource              = (*legacyPodBillingDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*legacyPodBillingDataSource)(nil)
)

func NewLegacyPodBillingDataSource() datasource.DataSource { return &legacyPodBillingDataSource{} }

type legacyPodBillingDataSource struct {
	client *legacy.Client
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

func (m legacyPodBillingModel) query() legacy.PodBillingQueryV1 {
	return legacy.PodBillingQueryV1{
		BucketSize: m.BucketSize.ValueString(),
		EndTime:    m.EndTime.ValueString(),
		GPUTypeID:  m.GpuTypeId.ValueString(),
		PodID:      m.PodId.ValueString(),
	}
}

func (d *legacyPodBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing_pod"
}

func (d *legacyPodBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deprecated. Pod spend as the 1.x provider reported it, read from the REST v1 billing/pods route. " +
			"Use runpod_pod_billing instead; this one stays only for the per-record fields REST v2 billing does not carry yet.",
		DeprecationMessage: legacyBillingDeprecation,
		Attributes: withAttributes(legacyBillingWindowAttributes(), map[string]schema.Attribute{
			"gpu_type_id": schema.StringAttribute{
				Optional:    true,
				Description: "Restrict the records to pods running one GPU type. REST v2 billing has no GPU type filter.",
			},
			"pod_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one pod."},
			"billing_records": legacyBillingRecords(map[string]schema.Attribute{
				"disk_space_billed_gb": schema.Int64Attribute{Computed: true, Description: "Disk space billed in GB."},
				"endpoint_id":          schema.StringAttribute{Computed: true, Description: "Endpoint the pod served, when it was a Serverless worker."},
				"gpu_type_id":          schema.StringAttribute{Computed: true, Description: "GPU type the pod ran on."},
				"pod_id":               schema.StringAttribute{Computed: true, Description: "The pod this record bills."},
				"time_billed_ms":       schema.Int64Attribute{Computed: true, Description: "Time billed in milliseconds."},
			}),
		}),
	}
}

func (d *legacyPodBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

func (d *legacyPodBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state legacyPodBillingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		appendMissingLegacyClient(&resp.Diagnostics, "pod billing")
		return
	}

	records, err := d.client.ListPodBillingV1(ctx, state.query())
	if err != nil {
		appendLegacyBillingFailure(&resp.Diagnostics, "pod billing", err)
		return
	}

	state.BillingRecords = flattenLegacyPodBilling(records)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenLegacyPodBilling(records []legacy.PodBillingRecordV1) []legacyPodBillingRecordModel {
	models := make([]legacyPodBillingRecordModel, 0, len(records))
	for _, record := range records {
		models = append(models, legacyPodBillingRecordModel{
			Amount:            types.Float64PointerValue(record.Amount),
			DiskSpaceBilledGb: legacyTruncatedInt64(record.DiskSpaceBilledGb),
			EndpointId:        types.StringPointerValue(record.EndpointID),
			GpuTypeId:         types.StringPointerValue(record.GPUTypeID),
			PodId:             types.StringPointerValue(record.PodID),
			Time:              types.StringPointerValue(record.Time),
			TimeBilledMs:      legacyTruncatedInt64(record.TimeBilledMs),
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
	client *legacy.Client
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

func (m legacyNetworkVolumeBillingModel) query() legacy.NetworkVolumeBillingQueryV1 {
	return legacy.NetworkVolumeBillingQueryV1{
		BucketSize:      m.BucketSize.ValueString(),
		EndTime:         m.EndTime.ValueString(),
		NetworkVolumeID: m.NetworkVolumeId.ValueString(),
	}
}

func (d *legacyNetworkVolumeBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing_network_volume"
}

func (d *legacyNetworkVolumeBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deprecated. Network volume spend as the 1.x provider reported it, read from the REST v1 " +
			"billing/networkvolumes route. Use runpod_network_volume_billing instead.",
		DeprecationMessage: legacyBillingDeprecation,
		Attributes: withAttributes(legacyBillingWindowAttributes(), map[string]schema.Attribute{
			"network_volume_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one network volume."},
			"billing_records": legacyBillingRecords(map[string]schema.Attribute{
				"disk_space_billed_gb": schema.Int64Attribute{Computed: true, Description: "Disk space billed in GB."},
				"network_volume_id":    schema.StringAttribute{Computed: true, Description: "The network volume this record bills."},
				"time_billed_ms":       schema.Int64Attribute{Computed: true, Description: "Time billed in milliseconds."},
			}),
		}),
	}
}

func (d *legacyNetworkVolumeBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

func (d *legacyNetworkVolumeBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state legacyNetworkVolumeBillingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		appendMissingLegacyClient(&resp.Diagnostics, "network volume billing")
		return
	}

	records, err := d.client.ListNetworkVolumeBillingV1(ctx, state.query())
	if err != nil {
		appendLegacyBillingFailure(&resp.Diagnostics, "network volume billing", err)
		return
	}

	state.BillingRecords = flattenLegacyNetworkVolumeBilling(records)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenLegacyNetworkVolumeBilling(records []legacy.NetworkVolumeBillingRecordV1) []legacyNetworkVolumeBillingRecordModel {
	models := make([]legacyNetworkVolumeBillingRecordModel, 0, len(records))
	for _, record := range records {
		models = append(models, legacyNetworkVolumeBillingRecordModel{
			Amount:            types.Float64PointerValue(record.Amount),
			DiskSpaceBilledGb: legacyTruncatedInt64(record.DiskSpaceBilledGb),
			NetworkVolumeId:   types.StringPointerValue(record.NetworkVolumeID),
			Time:              types.StringPointerValue(record.Time),
			TimeBilledMs:      legacyTruncatedInt64(record.TimeBilledMs),
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
	client *legacy.Client
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

func (m legacyEndpointBillingModel) query() legacy.EndpointBillingQueryV1 {
	return legacy.EndpointBillingQueryV1{
		BucketSize: m.BucketSize.ValueString(),
		EndTime:    m.EndTime.ValueString(),
		EndpointID: m.EndpointId.ValueString(),
	}
}

func (d *legacyEndpointBillingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing_endpoint"
}

func (d *legacyEndpointBillingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deprecated. Serverless endpoint spend as the 1.x provider reported it, read from the REST v1 " +
			"billing/endpoints route. Use runpod_serverless_billing instead.",
		DeprecationMessage: legacyBillingDeprecation,
		Attributes: withAttributes(legacyBillingWindowAttributes(), map[string]schema.Attribute{
			"endpoint_id": schema.StringAttribute{Optional: true, Description: "Restrict the records to one endpoint."},
			"billing_records": legacyBillingRecords(map[string]schema.Attribute{
				"endpoint_id":    schema.StringAttribute{Computed: true, Description: "The endpoint this record bills."},
				"time_billed_ms": schema.Int64Attribute{Computed: true, Description: "Time billed in milliseconds."},
			}),
		}),
	}
}

func (d *legacyEndpointBillingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

func (d *legacyEndpointBillingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state legacyEndpointBillingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		appendMissingLegacyClient(&resp.Diagnostics, "endpoint billing")
		return
	}

	records, err := d.client.ListEndpointBillingV1(ctx, state.query())
	if err != nil {
		appendLegacyBillingFailure(&resp.Diagnostics, "endpoint billing", err)
		return
	}

	state.BillingRecords = flattenLegacyEndpointBilling(records)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenLegacyEndpointBilling(records []legacy.EndpointBillingRecordV1) []legacyEndpointBillingRecordModel {
	models := make([]legacyEndpointBillingRecordModel, 0, len(records))
	for _, record := range records {
		models = append(models, legacyEndpointBillingRecordModel{
			Amount:       types.Float64PointerValue(record.Amount),
			EndpointId:   types.StringPointerValue(record.EndpointID),
			Time:         types.StringPointerValue(record.Time),
			TimeBilledMs: legacyTruncatedInt64(record.TimeBilledMs),
		})
	}
	return models
}
