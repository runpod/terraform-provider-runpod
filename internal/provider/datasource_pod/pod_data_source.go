package datasource_pod

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewPodDataSource() datasource.DataSource {
	return &PodDataSource{}
}

type PodDataSource struct {
	client *client.RunPodClient
}

func (d *PodDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *PodDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_pod"
}

func (d *PodDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = PodDataSourceSchema(ctx)
}

func (d *PodDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PodModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		query GetPod($podId: String!) {
			pod(input: { podId: $podId }) {
				id
				name
				status
				desiredStatus
				imageName
				machineId
				machineType
				gpuTypeId
				gpuCount
				costPerHr
				memoryInGb
				volumeInGb
				volumeMountPath
				volumeKey
				ports
				created_at
				dockerArgs
				env
				templateId
				containerDiskInGb
			}
		}
	`

variables := map[string]interface{}{
		"podId": config.Id.ValueString(),
	}

	if d.client == nil {
		resp.Diagnostics.AddError("Client not configured", "RunPod client is not configured")
		return
	}
	result, err := d.client.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if pod, ok := result["pod"].(map[string]interface{}); ok {
		envListValue := types.List{}
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		model := PodModel{
			Id:                config.Id,
			Name:              types.StringValue(pod["name"].(string)),
			Status:            types.StringValue(pod["status"].(string)),
			DesiredStatus:     types.StringValue(pod["desiredStatus"].(string)),
			ImageName:         types.StringValue(pod["imageName"].(string)),
			MachineId:         types.StringValue(pod["machineId"].(string)),
			MachineType:       types.StringValue(pod["machineType"].(string)),
			GpuTypeId:         types.StringValue(pod["gpuTypeId"].(string)),
			GpuCount:          types.Int64Value(int64(pod["gpuCount"].(float64))),
			CostPerHr:         types.Float64Value(pod["costPerHr"].(float64)),
			MemoryInGb:        types.Float64Value(pod["memoryInGb"].(float64)),
			VolumeInGb:        types.Float64Value(pod["volumeInGb"].(float64)),
			VolumeMountPath:   types.StringValue(pod["volumeMountPath"].(string)),
			Ports:             types.StringValue(pod["ports"].(string)),
			CreatedAt:         types.StringValue(pod["created_at"].(string)),
			DockerArgs:        types.StringValue(pod["dockerArgs"].(string)),
			Env:               envListValue,
			TemplateId:        types.StringValue(pod["templateId"].(string)),
			ContainerDiskInGb: types.Int64Value(int64(pod["containerDiskInGb"].(float64))),
		}
		diags = resp.State.Set(ctx, &model)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Pod not found in response")
	}
}
