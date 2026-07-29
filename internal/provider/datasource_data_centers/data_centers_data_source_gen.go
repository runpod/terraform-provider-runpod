// Schema/model definitions. Hand-maintained: terraform-provider-spec.json documents this
// logical schema, but the installed generator emits custom types for list_nested
// attributes, so this file is not byte-regenerable. Keep the spec in sync manually.

package datasource_data_centers

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func DataCentersDataSourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"data_centers": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "List of data centers",
				MarkdownDescription: "List of data centers",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"global_network": schema.BoolAttribute{
							Computed:            true,
							Description:         "Whether global network is enabled",
							MarkdownDescription: "Whether global network is enabled",
						},
						"id": schema.StringAttribute{
							Computed:            true,
							Description:         "Data center ID",
							MarkdownDescription: "Data center ID",
						},
						"location": schema.StringAttribute{
							Computed:            true,
							Description:         "Data center location",
							MarkdownDescription: "Data center location",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							Description:         "Data center name",
							MarkdownDescription: "Data center name",
						},
					},
				},
			},
		},
	}
}

type DataCentersModel struct {
	GlobalNetwork types.Bool   `tfsdk:"global_network"`
	Id            types.String `tfsdk:"id"`
	Location      types.String `tfsdk:"location"`
	Name          types.String `tfsdk:"name"`
}

type DataCentersDataSourceModel struct {
	DataCenters []DataCentersModel `tfsdk:"data_centers"`
}
