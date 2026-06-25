package datasource_billing_network_volume

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func BillingNetworkVolumeDataSourceSchema(ctx context.Context) schema.Schema {
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
			"network_volume_id": schema.StringAttribute{
				Optional:            true,
				Description:         "Filter by network volume ID",
				MarkdownDescription: "Filter billing records by network volume ID.",
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
						"network_volume_id": schema.StringAttribute{
							Computed:            true,
							Description:         "Network volume ID",
							MarkdownDescription: "Network volume ID (optional).",
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

type BillingNetworkVolumeModel struct {
	BucketSize      types.String         `tfsdk:"bucket_size"`
	EndTime         types.String         `tfsdk:"end_time"`
	NetworkVolumeId types.String         `tfsdk:"network_volume_id"`
	BillingRecords  []BillingRecordModel `tfsdk:"billing_records"`
}

type BillingRecordModel struct {
	Amount            types.Float64 `tfsdk:"amount"`
	DiskSpaceBilledGb types.Int64   `tfsdk:"disk_space_billed_gb"`
	NetworkVolumeId   types.String  `tfsdk:"network_volume_id"`
	Time              types.String  `tfsdk:"time"`
	TimeBilledMs      types.Int64   `tfsdk:"time_billed_ms"`
}
