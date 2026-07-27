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
	rlClient *client.RunPodClient
}

func (d *PodDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
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

	if d.rlClient == nil {
		resp.Diagnostics.AddError("Client not configured", "RunPod client is not configured")
		return
	}
	result, err := d.rlClient.Query(ctx, query, variables)
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
		
		var name, status, desiredStatus, imageName, machineId, machineType, gpuTypeId, ports, createdAt, dockerArgs, templateId, volumeMountPath string
		var gpuCount, costPerHr, memoryInGb, volumeInGb, containerDiskInGb float64
		
		if v, ok := pod["name"].(string); ok {
			name = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'name' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["status"].(string); ok {
			status = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'status' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["desiredStatus"].(string); ok {
			desiredStatus = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'desiredStatus' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["imageName"].(string); ok {
			imageName = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'imageName' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["machineId"].(string); ok {
			machineId = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'machineId' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["machineType"].(string); ok {
			machineType = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'machineType' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["gpuTypeId"].(string); ok {
			gpuTypeId = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'gpuTypeId' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["gpuCount"].(float64); ok {
			gpuCount = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'gpuCount' is missing or not a float64 in pod response")
			return
		}
		
		if v, ok := pod["costPerHr"].(float64); ok {
			costPerHr = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'costPerHr' is missing or not a float64 in pod response")
			return
		}
		
		if v, ok := pod["memoryInGb"].(float64); ok {
			memoryInGb = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'memoryInGb' is missing or not a float64 in pod response")
			return
		}
		
		if v, ok := pod["volumeInGb"].(float64); ok {
			volumeInGb = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'volumeInGb' is missing or not a float64 in pod response")
			return
		}
		
		if v, ok := pod["volumeMountPath"].(string); ok {
			volumeMountPath = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'volumeMountPath' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["ports"].(string); ok {
			ports = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'ports' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["created_at"].(string); ok {
			createdAt = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'created_at' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["dockerArgs"].(string); ok {
			dockerArgs = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'dockerArgs' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["templateId"].(string); ok {
			templateId = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'templateId' is missing or not a string in pod response")
			return
		}
		
		if v, ok := pod["containerDiskInGb"].(float64); ok {
			containerDiskInGb = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'containerDiskInGb' is missing or not a float64 in pod response")
			return
		}
		
		model := PodModel{
			Id:                config.Id,
			Name:              types.StringValue(name),
			Status:            types.StringValue(status),
			DesiredStatus:     types.StringValue(desiredStatus),
			ImageName:         types.StringValue(imageName),
			MachineId:         types.StringValue(machineId),
			MachineType:       types.StringValue(machineType),
			GpuTypeId:         types.StringValue(gpuTypeId),
			GpuCount:          types.Int64Value(int64(gpuCount)),
			CostPerHr:         types.Float64Value(costPerHr),
			MemoryInGb:        types.Float64Value(memoryInGb),
			VolumeInGb:        types.Float64Value(volumeInGb),
			VolumeMountPath:   types.StringValue(volumeMountPath),
			Ports:             types.StringValue(ports),
			CreatedAt:         types.StringValue(createdAt),
			DockerArgs:        types.StringValue(dockerArgs),
			Env:               envListValue,
			TemplateId:        types.StringValue(templateId),
			ContainerDiskInGb: types.Int64Value(int64(containerDiskInGb)),
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
