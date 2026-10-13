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
	_ datasource.DataSource              = (*cpuTypesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*cpuTypesDataSource)(nil)
)

func NewCpuTypesDataSource() datasource.DataSource { return &cpuTypesDataSource{} }

type cpuTypesDataSource struct {
	client *runpod.Client
}

type cpuTypeModel struct {
	Id                     types.String  `tfsdk:"id"`
	Name                   types.String  `tfsdk:"name"`
	Group                  types.String  `tfsdk:"group"`
	RamGbPerVcpu           types.Float64 `tfsdk:"ram_gb_per_vcpu"`
	VcpuMin                types.Int64   `tfsdk:"vcpu_min"`
	VcpuMax                types.Int64   `tfsdk:"vcpu_max"`
	PriceSecurePerVcpu     types.Float64 `tfsdk:"price_secure_per_vcpu"`
	PriceServerlessPerVcpu types.Float64 `tfsdk:"price_serverless_per_vcpu"`
}

type cpuTypesDataSourceModel struct {
	CpuTypes []cpuTypeModel `tfsdk:"cpu_types"`
}

func (d *cpuTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cpu_types"
}

func (d *cpuTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "CPU flavors offered by Runpod, from the catalog API. The flavor ids feed runpod_pod.cpu_type_id for a CPU-only pod and runpod_serverless_endpoint.cpu for a CPU-only endpoint.",
		Attributes: map[string]schema.Attribute{
			"cpu_types": schema.ListNestedAttribute{
				Computed:    true,
				Description: "All CPU flavors with their specs and pricing.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                        schema.StringAttribute{Computed: true, Description: "CPU flavor identifier."},
						"name":                      schema.StringAttribute{Computed: true, Description: "Human-readable CPU name."},
						"group":                     schema.StringAttribute{Computed: true, Description: "Flavor group."},
						"ram_gb_per_vcpu":           schema.Float64Attribute{Computed: true, Description: "RAM in GB per vCPU."},
						"vcpu_min":                  schema.Int64Attribute{Computed: true, Description: "Minimum vCPU count."},
						"vcpu_max":                  schema.Int64Attribute{Computed: true, Description: "Maximum vCPU count."},
						"price_secure_per_vcpu":     schema.Float64Attribute{Computed: true, Description: "Secure cloud price in USD/hr per vCPU."},
						"price_serverless_per_vcpu": schema.Float64Attribute{Computed: true, Description: "Serverless price in USD/hr per vCPU."},
					},
				},
			},
		},
	}
}

func (d *cpuTypesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *cpuTypesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apiResp, err := d.client.Catalog.ListCpuTypesWithResponse(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Listing CPU types failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Listing CPU types failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	state := cpuTypesDataSourceModel{CpuTypes: flattenCpuTypes(apiResp.JSON200.Cpus)}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
