// Schema/model definitions. Hand-maintained: terraform-provider-spec.json documents this
// logical schema, but the installed generator emits custom types for list_nested
// attributes, so this file is not byte-regenerable. Keep the spec in sync manually.

package datasource_gpu_types

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func GpuTypesDataSourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"gpu_types": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "List of GPU types",
				MarkdownDescription: "List of GPU types",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"community_price": schema.Float64Attribute{
							Computed:            true,
							Description:         "Community cloud price",
							MarkdownDescription: "Community cloud price",
						},
						"cuda_cores": schema.Int64Attribute{
							Computed:            true,
							Description:         "CUDA core count",
							MarkdownDescription: "CUDA core count",
						},
						"display_name": schema.StringAttribute{
							Computed:            true,
							Description:         "GPU display name",
							MarkdownDescription: "GPU display name",
						},
						"id": schema.StringAttribute{
							Computed:            true,
							Description:         "GPU type ID",
							MarkdownDescription: "GPU type ID",
						},
						"manufacturer": schema.StringAttribute{
							Computed:            true,
							Description:         "GPU manufacturer",
							MarkdownDescription: "GPU manufacturer",
						},
						"memory_in_gb": schema.Int64Attribute{
							Computed:            true,
							Description:         "Memory in GB",
							MarkdownDescription: "Memory in GB",
						},
						"secure_cloud": schema.BoolAttribute{
							Computed:            true,
							Description:         "Available in secure cloud",
							MarkdownDescription: "Available in secure cloud",
						},
						"secure_price": schema.Float64Attribute{
							Computed:            true,
							Description:         "Secure cloud price",
							MarkdownDescription: "Secure cloud price",
						},
					},
				},
			},
		},
	}
}

type GpuTypesModel struct {
	CommunityPrice types.Float64 `tfsdk:"community_price"`
	CudaCores      types.Int64   `tfsdk:"cuda_cores"`
	DisplayName    types.String  `tfsdk:"display_name"`
	Id             types.String  `tfsdk:"id"`
	Manufacturer   types.String  `tfsdk:"manufacturer"`
	MemoryInGb     types.Int64   `tfsdk:"memory_in_gb"`
	SecureCloud    types.Bool    `tfsdk:"secure_cloud"`
	SecurePrice    types.Float64 `tfsdk:"secure_price"`
}

type GpuTypesDataSourceModel struct {
	GpuTypes []GpuTypesModel `tfsdk:"gpu_types"`
}
