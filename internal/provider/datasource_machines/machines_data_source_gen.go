// Schema/model definitions. Hand-maintained: terraform-provider-spec.json documents this
// logical schema, but the installed generator emits custom types for list_nested
// attributes, so this file is not byte-regenerable. Keep the spec in sync manually.

package datasource_machines

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func MachinesDataSourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"machines": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "List of machines",
				MarkdownDescription: "List of machines",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"data_center_id": schema.StringAttribute{
							Computed:            true,
							Description:         "Data center ID",
							MarkdownDescription: "Data center ID",
						},
						"gpu_total": schema.Int64Attribute{
							Computed:            true,
							Description:         "Total GPU count",
							MarkdownDescription: "Total GPU count",
						},
						"gpu_type_id": schema.StringAttribute{
							Computed:            true,
							Description:         "GPU type ID",
							MarkdownDescription: "GPU type ID",
						},
						"id": schema.StringAttribute{
							Computed:            true,
							Description:         "Machine ID",
							MarkdownDescription: "Machine ID",
						},
						"listed": schema.BoolAttribute{
							Computed:            true,
							Description:         "Whether machine is listed",
							MarkdownDescription: "Whether machine is listed",
						},
						"location": schema.StringAttribute{
							Computed:            true,
							Description:         "Location",
							MarkdownDescription: "Location",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							Description:         "Machine name",
							MarkdownDescription: "Machine name",
						},
						"secure_cloud": schema.BoolAttribute{
							Computed:            true,
							Description:         "Whether machine is in secure cloud",
							MarkdownDescription: "Whether machine is in secure cloud",
						},
					},
				},
			},
		},
	}
}

type MachinesModel struct {
	DataCenterId types.String `tfsdk:"data_center_id"`
	GpuTotal     types.Int64  `tfsdk:"gpu_total"`
	GpuTypeId    types.String `tfsdk:"gpu_type_id"`
	Id           types.String `tfsdk:"id"`
	Listed       types.Bool   `tfsdk:"listed"`
	Location     types.String `tfsdk:"location"`
	Name         types.String `tfsdk:"name"`
	SecureCloud  types.Bool   `tfsdk:"secure_cloud"`
}

type MachinesDataSourceModel struct {
	Machines []MachinesModel `tfsdk:"machines"`
}
