package resource_machine

import (
	"os"
	"context"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewMachineResource() resource.Resource {
	return &MachineResource{}
}

type MachineResource struct{}

func (r *MachineResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_machine"
}

func (r *MachineResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = MachineResourceSchema(ctx)
}

func (r *MachineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config MachineModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		mutation CreateMachine($input: CreateMachineInput!) {
			machineCreate(input: $input) {
				id
				name
				description
				gpuCount
				gpuType
				cpuCount
				memoryInGb
				diskSizeInGb
				region
				priceHourly
				priceMonthly
				status
				createdAt
				updatedAt
			}
		}
	`

variables := map[string]interface{}{
			"input": map[string]interface{}{
				"name":         config.Name.ValueString(),
				"description":  config.Name.ValueString(),
				"gpuCount":     config.GpuCount.ValueInt64(),
				"gpuType":      config.GpuTypeId.ValueString(),
				"cpuCount":     config.CpuCount.ValueInt64(),
				"memoryInGb":   config.MemoryInGb.ValueInt64(),
				"diskSizeInGb": config.DiskInGb.ValueInt64(),
				"region":       config.Location.ValueString(),
				"secureCloud":  config.SecureCloud.ValueBool(),
				"listed":       config.Listed.ValueBool(),
			},
		}

	apiKey := os.Getenv("RUNPOD_API_KEY")
	runpodClient := client.NewRunPodClient(apiKey, client.GetGraphQLEndpoint())
	result, err := runpodClient.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		if machineCreate, ok := data["machineCreate"].(map[string]interface{}); ok {
			if machineID, ok := machineCreate["id"].(string); ok {
				config.Id = types.StringValue(machineID)
			} else {
				resp.Diagnostics.AddError("API Error", "Failed to get machine ID from response")
				return
			}
		} else {
			resp.Diagnostics.AddError("API Error", "machineCreate not in response")
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "data not in response")
		return
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *MachineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var config MachineModel
	diags := req.State.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		query GetMachine($machineId: String!) {
			machine(input: { machineId: $machineId }) {
				id
				name
				description
				gpuCount
				gpuType
				cpuCount
				memoryInGb
				diskSizeInGb
				region
				priceHourly
				priceMonthly
				status
				createdAt
				updatedAt
				listed
				location
				secureCloud
				maintenanceMode
				verified
				hostPricePerGpu
				diskTotal
				diskReserved
				memoryTotal
				memoryReserved
				gpuTotal
				gpuReserved
				cpuTypeId
				runpodIp
			}
		}
	`

variables := map[string]interface{}{
			"machineId": config.Id.ValueString(),
		}

	apiKey := os.Getenv("RUNPOD_API_KEY")
	runpodClient := client.NewRunPodClient(apiKey, client.GetGraphQLEndpoint())
	result, err := runpodClient.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		if machine, ok := data["machine"].(map[string]interface{}); ok {
			config.Name = types.StringValue(machine["name"].(string))
			config.GpuCount = types.Int64Value(int64(machine["gpuCount"].(float64)))
			config.GpuTypeId = types.StringValue(machine["gpuType"].(string))
			config.CpuCount = types.Int64Value(int64(machine["cpuCount"].(float64)))
			config.MemoryInGb = types.Int64Value(int64(machine["memoryInGb"].(float64)))
			config.DiskInGb = types.Int64Value(int64(machine["diskSizeInGb"].(float64)))
			config.Location = types.StringValue(machine["region"].(string))
			config.Listed = types.BoolValue(machine["listed"].(bool))
			config.SecureCloud = types.BoolValue(machine["secureCloud"].(bool))
			config.MaintenanceMode = types.BoolValue(machine["maintenanceMode"].(bool))
			config.Verified = types.BoolValue(machine["verified"].(bool))
			config.HostPricePerGpu = types.Float64Value(machine["hostPricePerGpu"].(float64))
		} else {
			resp.Diagnostics.AddError("API Error", "Machine not found in response")
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to get data from response")
		return
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *MachineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config MachineModel
	diags := req.Plan.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		mutation EditMachine($input: EditMachineInput!) {
			machineEdit(input: $input) {
				id
				name
				description
				gpuCount
				gpuType
				cpuCount
				memoryInGb
				diskSizeInGb
				region
				listed
			}
		}
	`

variables := map[string]interface{}{
			"input": map[string]interface{}{
				"id":           config.Id.ValueString(),
				"name":         config.Name.ValueString(),
				"description":  config.Name.ValueString(),
				"gpuCount":     config.GpuCount.ValueInt64(),
				"gpuType":      config.GpuTypeId.ValueString(),
				"cpuCount":     config.CpuCount.ValueInt64(),
				"memoryInGb":   config.MemoryInGb.ValueInt64(),
				"diskSizeInGb": config.DiskInGb.ValueInt64(),
				"region":       config.Location.ValueString(),
				"listed":       config.Listed.ValueBool(),
			},
		}

	apiKey := os.Getenv("RUNPOD_API_KEY")
	runpodClient := client.NewRunPodClient(apiKey, client.GetGraphQLEndpoint())
	_, err := runpodClient.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *MachineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var config MachineModel
	diags := req.State.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		mutation DeleteMachine($machineId: String!) {
			machineDelete(machineId: $machineId) {
				id
				status
			}
		}
	`

variables := map[string]interface{}{
			"machineId": config.Id.ValueString(),
		}

	apiKey := os.Getenv("RUNPOD_API_KEY")
	runpodClient := client.NewRunPodClient(apiKey, client.GetGraphQLEndpoint())
	_, err := runpodClient.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}
}
