package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
)

var (
	_ datasource.DataSource              = (*gpuTypesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*gpuTypesDataSource)(nil)
)

func NewGpuTypesDataSource() datasource.DataSource { return &gpuTypesDataSource{} }

type gpuTypesDataSource struct {
	client *runpod.Client
}

type gpuTypeModel struct {
	Id             types.String  `tfsdk:"id"`
	Name           types.String  `tfsdk:"name"`
	Pool           types.String  `tfsdk:"pool"`
	Memory         types.Int64   `tfsdk:"memory"`
	Secure         types.Bool    `tfsdk:"secure"`
	Community      types.Bool    `tfsdk:"community"`
	PriceSecure    types.Float64 `tfsdk:"price_secure"`
	PriceCommunity types.Float64 `tfsdk:"price_community"`
}

type gpuTypesDataSourceModel struct {
	GpuTypes []gpuTypeModel `tfsdk:"gpu_types"`
}

func (d *gpuTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gpu_types"
}

func (d *gpuTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "GPU types available for pod creation, from the catalog API.",
		Attributes: map[string]schema.Attribute{
			"gpu_types": schema.ListNestedAttribute{
				Computed:    true,
				Description: "All GPU types with their specs, pricing, and data-center availability.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":              schema.StringAttribute{Computed: true, Description: "GPU type identifier, e.g. \"NVIDIA GeForce RTX 4090\"."},
						"name":            schema.StringAttribute{Computed: true, Description: "Human-readable GPU name."},
						"pool":            schema.StringAttribute{Computed: true, Description: "Serverless GPU pool ID for runpod_serverless_endpoint.gpu_pools; null when not in a pool."},
						"memory":          schema.Int64Attribute{Computed: true, Description: "VRAM in GB."},
						"secure":          schema.BoolAttribute{Computed: true, Description: "Available on secure cloud."},
						"community":       schema.BoolAttribute{Computed: true, Description: "Available on community cloud."},
						"price_secure":    schema.Float64Attribute{Computed: true, Description: "Secure cloud price in USD/hr."},
						"price_community": schema.Float64Attribute{Computed: true, Description: "Community cloud price in USD/hr."},
					},
				},
			},
		},
	}
}

func (d *gpuTypesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *gpuTypesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apiResp, err := d.client.Catalog.ListGpuTypesWithResponse(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Listing GPU types failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Listing GPU types failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	state := gpuTypesDataSourceModel{GpuTypes: flattenGpuTypes(apiResp.JSON200.Gpus)}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
