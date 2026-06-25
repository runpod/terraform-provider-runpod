package datasource_billing_pod

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func BillingPodDataSourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"bucket_size": schema.StringAttribute{
				Optional:            true,
				Description:         "Billing bucket size: hour, day, week, month, year",
				MarkdownDescription: "Billing bucket size: hour, day, week, month, year. Defaults to day.",
			},
			"end_time": schema.StringAttribute{
				Optional:            true,
				Description:         "End time for billing query (ISO datetime)",
				MarkdownDescription: "End time for billing query (ISO datetime). Defaults to current time.",
			},
			"gpu_type_id": schema.StringAttribute{
				Optional:            true,
				Description:         "Filter by GPU type ID",
				MarkdownDescription: "Filter billing records by GPU type ID.",
			},
			"pod_id": schema.StringAttribute{
				Optional:            true,
				Description:         "Filter by pod ID",
				MarkdownDescription: "Filter billing records by pod ID.",
			},
			"billing_records": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "List of billing records",
				MarkdownDescription: "List of billing records.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"amount": schema.Float64Attribute{
							Computed:            true,
							Description:         "Amount charged in USD",
							MarkdownDescription: "Amount charged in USD.",
						},
						"disk_space_billed_gb": schema.Int64Attribute{
							Computed:            true,
							Description:         "Disk space billed in GB",
							MarkdownDescription: "Disk space billed in GB (optional).",
						},
						"endpoint_id": schema.StringAttribute{
							Computed:            true,
							Description:         "Endpoint ID (if applicable)",
							MarkdownDescription: "Endpoint ID if this record is for an endpoint (optional).",
						},
						"gpu_type_id": schema.StringAttribute{
							Computed:            true,
							Description:         "GPU type ID",
							MarkdownDescription: "GPU type ID (optional).",
						},
						"pod_id": schema.StringAttribute{
							Computed:            true,
							Description:         "Pod ID",
							MarkdownDescription: "Pod ID (optional).",
						},
						"time": schema.StringAttribute{
							Computed:            true,
							Description:         "Billing time (ISO datetime)",
							MarkdownDescription: "Billing time in ISO datetime format.",
						},
						"time_billed_ms": schema.Int64Attribute{
							Computed:            true,
							Description:         "Time billed in milliseconds",
							MarkdownDescription: "Time billed in milliseconds (optional).",
						},
					},
				},
			},
		},
	}
}

type BillingPodModel struct {
	BucketSize     types.String         `tfsdk:"bucket_size"`
	EndTime        types.String         `tfsdk:"end_time"`
	GpuTypeId      types.String         `tfsdk:"gpu_type_id"`
	PodId          types.String         `tfsdk:"pod_id"`
	BillingRecords []BillingRecordModel `tfsdk:"billing_records"`
}

type BillingRecordModel struct {
	Amount            types.Float64 `tfsdk:"amount"`
	DiskSpaceBilledGb types.Int64   `tfsdk:"disk_space_billed_gb"`
	EndpointId        types.String  `tfsdk:"endpoint_id"`
	GpuTypeId         types.String  `tfsdk:"gpu_type_id"`
	PodId             types.String  `tfsdk:"pod_id"`
	Time              types.String  `tfsdk:"time"`
	TimeBilledMs      types.Int64   `tfsdk:"time_billed_ms"`
}
