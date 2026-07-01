package datasource_gpu_types

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewGpuTypesDataSource() datasource.DataSource {
	return &GpuTypesDataSource{}
}

type GpuTypesDataSource struct {
	client *client.RunPodClient
}

func (d *GpuTypesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *GpuTypesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_gpu_types"
}

func (d *GpuTypesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = GpuTypesDataSourceSchema(ctx)
}

func (d *GpuTypesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	query := `
		query GetGpuTypes {
			gpus {
				id
				displayName
				manufacturer
				cuda_cores
				memory_in_gb
				community_price
				secure_price
				secure_cloud
			}
		}
	`

	variables := map[string]interface{}{}

	if d.client == nil {
		resp.Diagnostics.AddError("Client not configured", "RunPod client is not configured")
		return
	}
	result, err := d.client.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if gpus, ok := result["gpus"].([]interface{}); ok {
		models := make([]GpuTypesModel, len(gpus))
		for i, gpu := range gpus {
			if gpuMap, ok := gpu.(map[string]interface{}); ok {
				models[i] = GpuTypesModel{
					Id:             types.StringValue(gpuMap["id"].(string)),
					DisplayName:    types.StringValue(gpuMap["displayName"].(string)),
					Manufacturer:   types.StringValue(gpuMap["manufacturer"].(string)),
					CudaCores:      types.Int64Value(int64(gpuMap["cuda_cores"].(float64))),
					MemoryInGb:     types.Int64Value(int64(gpuMap["memory_in_gb"].(float64))),
					CommunityPrice: types.Float64Value(gpuMap["community_price"].(float64)),
					SecurePrice:    types.Float64Value(gpuMap["secure_price"].(float64)),
					SecureCloud:    types.BoolValue(gpuMap["secure_cloud"].(bool)),
				}
			}
		}
		diags := resp.State.Set(ctx, &models)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to parse GPU types from response")
	}
}
