package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
)

// Singular by-id lookups over the catalog: runpod_gpu_type, runpod_cpu_type,
// runpod_data_center. Each reuses the corresponding list item model, so the
// attribute descriptions match the list data sources.

var (
	_ datasource.DataSource              = (*gpuTypeDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*gpuTypeDataSource)(nil)
)

func NewGpuTypeDataSource() datasource.DataSource { return &gpuTypeDataSource{} }

type gpuTypeDataSource struct {
	client *runpod.Client
}

func (d *gpuTypeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gpu_type"
}

func (d *gpuTypeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A single GPU type looked up by identifier.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Required: true, Description: "GPU type identifier, e.g. \"NVIDIA GeForce RTX 4090\"."},
			"name":            schema.StringAttribute{Computed: true, Description: "Human-readable GPU name."},
			"pool":            schema.StringAttribute{Computed: true, Description: "Serverless GPU pool ID for runpod_serverless_endpoint.gpu_pools; null when not in a pool."},
			"memory":          schema.Int64Attribute{Computed: true, Description: "VRAM in GB."},
			"secure":          schema.BoolAttribute{Computed: true, Description: "Available on secure cloud."},
			"community":       schema.BoolAttribute{Computed: true, Description: "Available on community cloud."},
			"price_secure":    schema.Float64Attribute{Computed: true, Description: "Secure cloud price in USD/hr."},
			"price_community": schema.Float64Attribute{Computed: true, Description: "Community cloud price in USD/hr."},
		},
	}
}

func (d *gpuTypeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *gpuTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config gpuTypeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Catalog.GetGpuTypeWithResponse(ctx, config.Id.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading GPU type failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading GPU type failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	state := flattenGpuType(*apiResp.JSON200)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

var (
	_ datasource.DataSource              = (*cpuTypeDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*cpuTypeDataSource)(nil)
)

func NewCpuTypeDataSource() datasource.DataSource { return &cpuTypeDataSource{} }

type cpuTypeDataSource struct {
	client *runpod.Client
}

func (d *cpuTypeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cpu_type"
}

func (d *cpuTypeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A single CPU flavor looked up by identifier. The flavor id feeds runpod_pod.cpu_type_id for a CPU-only pod and runpod_serverless_endpoint.cpu for a CPU-only endpoint.",
		Attributes: map[string]schema.Attribute{
			"id":                        schema.StringAttribute{Required: true, Description: "CPU flavor identifier, e.g. \"cpu3c\"."},
			"name":                      schema.StringAttribute{Computed: true, Description: "Human-readable CPU name."},
			"group":                     schema.StringAttribute{Computed: true, Description: "Flavor group."},
			"ram_gb_per_vcpu":           schema.Float64Attribute{Computed: true, Description: "RAM in GB per vCPU."},
			"vcpu_min":                  schema.Int64Attribute{Computed: true, Description: "Minimum vCPU count."},
			"vcpu_max":                  schema.Int64Attribute{Computed: true, Description: "Maximum vCPU count."},
			"price_secure_per_vcpu":     schema.Float64Attribute{Computed: true, Description: "Secure cloud price in USD/hr per vCPU."},
			"price_serverless_per_vcpu": schema.Float64Attribute{Computed: true, Description: "Serverless price in USD/hr per vCPU."},
		},
	}
}

func (d *cpuTypeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *cpuTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cpuTypeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Catalog.GetCpuTypeWithResponse(ctx, config.Id.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading CPU type failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading CPU type failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	state := flattenCpuType(*apiResp.JSON200)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

var (
	_ datasource.DataSource              = (*dataCenterDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*dataCenterDataSource)(nil)
)

func NewDataCenterDataSource() datasource.DataSource { return &dataCenterDataSource{} }

type dataCenterDataSource struct {
	client *runpod.Client
}

func (d *dataCenterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_data_center"
}

func (d *dataCenterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A single data center looked up by identifier.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Required: true, Description: "Data center identifier, e.g. \"EU-RO-1\"."},
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
	}
}

func (d *dataCenterDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *dataCenterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config dataCenterModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Catalog.GetDataCenterWithResponse(ctx, config.Id.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading data center failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading data center failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	state, diags := flattenDataCenter(ctx, *apiResp.JSON200)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
