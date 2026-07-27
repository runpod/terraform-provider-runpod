package datasource_endpoint_workers

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/attr"
)

func EndpointWorkersDataSourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"endpoint_id": schema.StringAttribute{
				Required: true,
			},
			"workers": schema.ListAttribute{
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"id":           types.StringType,
						"pod_id":       types.StringType,
						"status":       types.StringType,
						"uptime_ms":    types.Int64Type,
						"start_time":   types.StringType,
						"last_busy_ms": types.Int64Type,
					},
				},
				Computed: true,
			},
		},
	}
}

type EndpointWorkersDataSourceModel struct {
	Id         types.String `tfsdk:"id"`
	EndpointId types.String `tfsdk:"endpoint_id"`
	Workers    types.List   `tfsdk:"workers"`
}
