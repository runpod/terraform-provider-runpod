package datasource_billing_endpoint

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func BillingEndpointDataSourceSchema(ctx context.Context) schema.Schema {
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
			"endpoint_id": schema.StringAttribute{
				Optional:            true,
				Description:         "Filter by endpoint ID",
				MarkdownDescription: "Filter billing records by endpoint ID.",
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
						"endpoint_id": schema.StringAttribute{
							Computed:            true,
							Description:         "Endpoint ID",
							MarkdownDescription: "Endpoint ID (optional).",
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

type BillingEndpointModel struct {
	BucketSize     types.String         `tfsdk:"bucket_size"`
	EndTime        types.String         `tfsdk:"end_time"`
	EndpointId     types.String         `tfsdk:"endpoint_id"`
	BillingRecords []BillingRecordModel `tfsdk:"billing_records"`
}

type BillingRecordModel struct {
	Amount       types.Float64 `tfsdk:"amount"`
	EndpointId   types.String  `tfsdk:"endpoint_id"`
	Time         types.String  `tfsdk:"time"`
	TimeBilledMs types.Int64   `tfsdk:"time_billed_ms"`
}
