package datasource_endpoint

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func EndpointDataSourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"image": schema.StringAttribute{
				Computed: true,
			},
			"gpu_pools": schema.StringAttribute{
				Computed: true,
			},
			"gpu_count": schema.Int64Attribute{
				Computed: true,
			},
			"workers_min": schema.Int64Attribute{
				Computed: true,
			},
			"workers_max": schema.Int64Attribute{
				Computed: true,
			},
			"scaler_type": schema.StringAttribute{
				Computed: true,
			},
			"scaler_value": schema.Float64Attribute{
				Computed: true,
			},
			"idle_timeout": schema.Int64Attribute{
				Computed: true,
			},
			"run_url": schema.StringAttribute{
				Computed: true,
			},
			"run_sync_url": schema.StringAttribute{
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				Computed: true,
			},
			"data_center_ids": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

type EndpointDataSourceModel struct {
	Id           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Image        types.String `tfsdk:"image"`
	GpuPools     types.String `tfsdk:"gpu_pools"`
	GpuCount     types.Int64  `tfsdk:"gpu_count"`
	WorkersMin   types.Int64  `tfsdk:"workers_min"`
	WorkersMax   types.Int64  `tfsdk:"workers_max"`
	ScalerType   types.String `tfsdk:"scaler_type"`
	ScalerValue  types.Float64 `tfsdk:"scaler_value"`
	IdleTimeout  types.Int64  `tfsdk:"idle_timeout"`
	RunUrl       types.String `tfsdk:"run_url"`
	RunSyncUrl   types.String `tfsdk:"run_sync_url"`
	CreatedAt    types.String `tfsdk:"created_at"`
	DataCenterIds types.String `tfsdk:"data_center_ids"`
}
