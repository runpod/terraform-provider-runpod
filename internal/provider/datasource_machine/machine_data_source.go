package datasource_machine

import (
	"os"
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewMachineDataSource() datasource.DataSource {
	return &MachineDataSource{}
}

type MachineDataSource struct {
	rlClient *client.RunPodClient
}

func (d *MachineDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *MachineDataSource) getClient() *client.RunPodClient {
	if d.rlClient != nil {
		return d.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	endpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if endpoint == "" {
		endpoint = "https://api.runpod.io/graphql"
	}
	// Machines are GraphQL-only; the REST base URL is unused here.
	d.rlClient = client.NewRunPodClient(apiKey, endpoint, "")
	return d.rlClient
}

func (d *MachineDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_machine"
}

func (d *MachineDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = MachineDataSourceSchema(ctx)
}

func (d *MachineDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config MachineModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		query GetMachine($machineId: String!) {
			machine(input: { machineId: $machineId }) {
				id
				name
				location
				listed
				gpuType {
					id
					displayName
				}
				gpuTotal
				gpuReserved
				cpuCount
				cpuTypeId
				memoryTotal
				memoryReserved
				diskTotal
				diskReserved
				secureCloud
				maintenanceMode
				verified
				hostPricePerGpu
				runpodIp
			}
		}
	`

variables := map[string]interface{}{
			"machineId": config.Id.ValueString(),
		}

	rlClient := d.getClient()
	result, err := rlClient.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if machine, ok := result["machine"].(map[string]interface{}); ok {
		gpuTypeMap, ok := machine["gpuType"].(map[string]interface{})
		gpuTypeId := types.StringValue("")
		if ok {
			if id, ok := gpuTypeMap["id"].(string); ok {
				gpuTypeId = types.StringValue(id)
			}
		}
		
		var name, location, cpuTypeId, runpodIp string
		var listed, secureCloud, maintenanceMode, verified bool
		var gpuTotal, gpuReserved, cpuCount, memoryTotal, memoryReserved, diskTotal, diskReserved float64
		var hostPricePerGpu float64
		
		if v, ok := machine["name"].(string); ok {
			name = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'name' is missing or not a string in machine response")
			return
		}
		
		if v, ok := machine["location"].(string); ok {
			location = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'location' is missing or not a string in machine response")
			return
		}
		
		if v, ok := machine["listed"].(bool); ok {
			listed = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'listed' is missing or not a bool in machine response")
			return
		}
		
		if v, ok := machine["gpuTotal"].(float64); ok {
			gpuTotal = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'gpuTotal' is missing or not a float64 in machine response")
			return
		}
		
		if v, ok := machine["gpuReserved"].(float64); ok {
			gpuReserved = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'gpuReserved' is missing or not a float64 in machine response")
			return
		}
		
		if v, ok := machine["cpuCount"].(float64); ok {
			cpuCount = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'cpuCount' is missing or not a float64 in machine response")
			return
		}
		
		if v, ok := machine["cpuTypeId"].(string); ok {
			cpuTypeId = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'cpuTypeId' is missing or not a string in machine response")
			return
		}
		
		if v, ok := machine["memoryTotal"].(float64); ok {
			memoryTotal = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'memoryTotal' is missing or not a float64 in machine response")
			return
		}
		
		if v, ok := machine["memoryReserved"].(float64); ok {
			memoryReserved = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'memoryReserved' is missing or not a float64 in machine response")
			return
		}
		
		if v, ok := machine["diskTotal"].(float64); ok {
			diskTotal = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'diskTotal' is missing or not a float64 in machine response")
			return
		}
		
		if v, ok := machine["diskReserved"].(float64); ok {
			diskReserved = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'diskReserved' is missing or not a float64 in machine response")
			return
		}
		
		if v, ok := machine["secureCloud"].(bool); ok {
			secureCloud = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'secureCloud' is missing or not a bool in machine response")
			return
		}
		
		if v, ok := machine["maintenanceMode"].(bool); ok {
			maintenanceMode = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'maintenanceMode' is missing or not a bool in machine response")
			return
		}
		
		if v, ok := machine["verified"].(bool); ok {
			verified = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'verified' is missing or not a bool in machine response")
			return
		}
		
		if v, ok := machine["hostPricePerGpu"].(float64); ok {
			hostPricePerGpu = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'hostPricePerGpu' is missing or not a float64 in machine response")
			return
		}
		
		if v, ok := machine["runpodIp"].(string); ok {
			runpodIp = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'runpodIp' is missing or not a string in machine response")
			return
		}
		
		model := MachineModel{
			Id:              config.Id,
			Name:            types.StringValue(name),
			Location:        types.StringValue(location),
			Listed:          types.BoolValue(listed),
			GpuTypeId:       gpuTypeId,
			GpuTotal:        types.Int64Value(int64(gpuTotal)),
			GpuReserved:     types.Int64Value(int64(gpuReserved)),
			CpuCount:        types.Int64Value(int64(cpuCount)),
			CpuTypeId:       types.StringValue(cpuTypeId),
			MemoryTotal:     types.Int64Value(int64(memoryTotal)),
			MemoryReserved:  types.Int64Value(int64(memoryReserved)),
			DiskTotal:       types.Int64Value(int64(diskTotal)),
			DiskReserved:    types.Int64Value(int64(diskReserved)),
			SecureCloud:     types.BoolValue(secureCloud),
			MaintenanceMode: types.BoolValue(maintenanceMode),
			Verified:        types.BoolValue(verified),
			HostPricePerGpu: types.Float64Value(hostPricePerGpu),
			RunpodIp:        types.StringValue(runpodIp),
		}
		diags = resp.State.Set(ctx, &model)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Machine not found in response")
	}
}
