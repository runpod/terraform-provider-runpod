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
	_ datasource.DataSource              = (*dataCentersDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*dataCentersDataSource)(nil)
)

func NewDataCentersDataSource() datasource.DataSource { return &dataCentersDataSource{} }

type dataCentersDataSource struct {
	client *runpod.Client
}

type dataCenterModel struct {
	Id                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Region             types.String `tfsdk:"region"`
	GlobalNetwork      types.Bool   `tfsdk:"global_network"`
	Compliance         types.List   `tfsdk:"compliance"`
	NetworkVolumeTypes types.List   `tfsdk:"network_volume_types"`
}

type dataCentersDataSourceModel struct {
	DataCenters []dataCenterModel `tfsdk:"data_centers"`
}

func (d *dataCentersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_data_centers"
}

func (d *dataCentersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Runpod data centers, from the catalog API.",
		Attributes: map[string]schema.Attribute{
			"data_centers": schema.ListNestedAttribute{
				Computed:    true,
				Description: "All data centers with their location, compliance, and GPU availability.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, Description: "Data center identifier, e.g. \"US-TX-3\"."},
						"name":           schema.StringAttribute{Computed: true, Description: "Human-readable data center name."},
						"region":         schema.StringAttribute{Computed: true, Description: "Geographic region."},
						"global_network": schema.BoolAttribute{Computed: true, Description: "Whether global networking is available."},
						"compliance": schema.ListAttribute{
							ElementType: types.StringType,
							Computed:    true,
							Description: "Compliance certifications held by the data center.",
						},
						"network_volume_types": schema.ListAttribute{
							ElementType: types.StringType,
							Computed:    true,
							Description: "Network volume storage tiers offered.",
						},
					},
				},
			},
		},
	}
}

func (d *dataCentersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *dataCentersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apiResp, err := d.client.Catalog.ListDataCentersWithResponse(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Listing data centers failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Listing data centers failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	dataCenters, diags := flattenDataCenters(ctx, apiResp.JSON200.DataCenters)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state := dataCentersDataSourceModel{DataCenters: dataCenters}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
