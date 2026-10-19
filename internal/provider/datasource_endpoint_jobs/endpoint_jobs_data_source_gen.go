package datasource_endpoint_jobs

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func EndpointJobsDataSourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"endpoint_id": schema.StringAttribute{
				Required: true,
			},
			"status_filter": schema.StringAttribute{
				Optional: true,
			},
			"limit": schema.Int64Attribute{
				Optional: true,
			},
			"cursor": schema.StringAttribute{
				Optional: true,
			},
			"next_cursor": schema.StringAttribute{
				Computed: true,
			},
			"jobs": schema.ListAttribute{
				Computed: true,
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"id":           types.StringType,
						"status":       types.StringType,
						"created_at":   types.StringType,
						"duration_ms":  types.Int64Type,
						"completed_at": types.StringType,
						"input":        types.StringType,
						"output":       types.StringType,
					},
				},
			},
		},
	}
}

type EndpointJobsDataSourceModel struct {
	Id           types.String `tfsdk:"id"`
	EndpointId   types.String `tfsdk:"endpoint_id"`
	StatusFilter types.String `tfsdk:"status_filter"`
	Limit        types.Int64  `tfsdk:"limit"`
	Cursor       types.String `tfsdk:"cursor"`
	NextCursor   types.String `tfsdk:"next_cursor"`
	Jobs         types.List   `tfsdk:"jobs"`
}
