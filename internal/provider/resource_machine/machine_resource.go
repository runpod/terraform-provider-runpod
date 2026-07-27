package resource_machine

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewMachineResource() resource.Resource {
	return &MachineResource{}
}

type MachineResource struct {
	rlClient *client.RunPodClient
}

func (r *MachineResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		if clientWrapper, ok := req.ProviderData.(*client.RunPodClientWrapper); ok {
			r.rlClient = &client.RunPodClient{
				APIKey:      clientWrapper.APIKey,
				GraphQLEndpoint: "https://api.runpod.io/graphql",
				RestBaseURL: clientWrapper.RestBaseURL,
				Client: &http.Client{Timeout: 60 * time.Second},
			}
		} else if client, ok := req.ProviderData.(*client.RunPodClient); ok {
			r.rlClient = client
		}
	}
}

func (r *MachineResource) getClient() *client.RunPodClient {
	if r.rlClient != nil {
		return r.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	graphqlEndpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if graphqlEndpoint == "" {
		graphqlEndpoint = "https://api.runpod.io/graphql"
	}
	restBaseURL := os.Getenv("RUNPOD_BASE_URL")
	if restBaseURL == "" {
		restBaseURL = "https://rest.runpod.io/v1"
	}
	r.rlClient = client.NewRunPodClient(apiKey, graphqlEndpoint, restBaseURL)
	return r.rlClient
}

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
		mutation CreateMachine($input: MachineAddInput!) {
			machineAdd(input: $input) {
				id
				name
				gpuType {
					id
					displayName
				}
				cpuCount
				gpuTotal
				memoryTotal
				diskTotal
				listed
				secureCloud
				location
			}
		}
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"name": config.Name.ValueString(),
		},
	}

	result, err := r.getClient().Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if machineAdd, ok := result["machineAdd"].(map[string]interface{}); ok {
		if machineID, ok := machineAdd["id"].(string); ok {
			config.Id = types.StringValue(machineID)
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to get machine ID from response")
			return
		}

		if v, ok := machineAdd["name"].(string); ok {
			config.Name = types.StringValue(v)
		}

		if gpuTypeMap, ok := machineAdd["gpuType"].(map[string]interface{}); ok {
			if id, ok := gpuTypeMap["id"].(string); ok {
				config.GpuTypeId = types.StringValue(id)
			}
		}

		if v, ok := machineAdd["cpuCount"].(float64); ok {
			config.CpuCount = types.Int64Value(int64(v))
		}

		if v, ok := machineAdd["gpuTotal"].(float64); ok {
			config.GpuCount = types.Int64Value(int64(v))
		}

		if v, ok := machineAdd["memoryTotal"].(float64); ok {
			config.MemoryInGb = types.Int64Value(int64(v))
		}

		if v, ok := machineAdd["diskTotal"].(float64); ok {
			config.DiskInGb = types.Int64Value(int64(v))
		}

		if v, ok := machineAdd["listed"].(bool); ok {
			config.Listed = types.BoolValue(v)
		}

		if v, ok := machineAdd["secureCloud"].(bool); ok {
			config.SecureCloud = types.BoolValue(v)
		}

		if v, ok := machineAdd["location"].(string); ok {
			config.Location = types.StringValue(v)
		}
	} else {
		resp.Diagnostics.AddError("API Error", "machineAdd not in response")
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
				gpuType {
					id
					displayName
				}
				cpuCount
				gpuTotal
				memoryTotal
				diskTotal
				listed
				secureCloud
				location
				maintenanceMode
				verified
				hostPricePerGpu
				dataCenterId
				runpodIp
			}
		}
	`

	variables := map[string]interface{}{
		"machineId": config.Id.ValueString(),
	}

	result, err := r.getClient().Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if machine, ok := result["machine"].(map[string]interface{}); ok {
		var name string

		if v, ok := machine["name"].(string); ok {
			name = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'name' is missing or not a string in machine response")
			return
		}

		if gpuTypeMap, ok := machine["gpuType"].(map[string]interface{}); ok {
			if id, ok := gpuTypeMap["id"].(string); ok {
				config.GpuTypeId = types.StringValue(id)
			}
		}

		if v, ok := machine["cpuCount"].(float64); ok {
			config.CpuCount = types.Int64Value(int64(v))
		}

		if v, ok := machine["gpuTotal"].(float64); ok {
			config.GpuCount = types.Int64Value(int64(v))
		}

		if v, ok := machine["memoryTotal"].(float64); ok {
			config.MemoryInGb = types.Int64Value(int64(v))
		}

		if v, ok := machine["diskTotal"].(float64); ok {
			config.DiskInGb = types.Int64Value(int64(v))
		}

		if v, ok := machine["listed"].(bool); ok {
			config.Listed = types.BoolValue(v)
		}

		if v, ok := machine["secureCloud"].(bool); ok {
			config.SecureCloud = types.BoolValue(v)
		}

		if v, ok := machine["location"].(string); ok {
			config.Location = types.StringValue(v)
		}

		if v, ok := machine["maintenanceMode"].(bool); ok {
			config.MaintenanceMode = types.BoolValue(v)
		}

		if v, ok := machine["verified"].(bool); ok {
			config.Verified = types.BoolValue(v)
		}

		if v, ok := machine["hostPricePerGpu"].(float64); ok {
			config.HostPricePerGpu = types.Float64Value(v)
		}

		if v, ok := machine["dataCenterId"].(string); ok {
			config.DataCenterId = types.StringValue(v)
		}

		if v, ok := machine["runpodIp"].(string); ok {
			config.RunpodIp = types.StringValue(v)
		}

		config.Name = types.StringValue(name)
	} else {
		resp.Diagnostics.AddError("API Error", "Machine not found in response")
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
		mutation EditMachineName($input: MachineEditNameInput!) {
			machineEditName(input: $input) {
				id
				name
			}
		}
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"id":   config.Id.ValueString(),
			"name": config.Name.ValueString(),
		},
	}

	_, err := r.getClient().Query(ctx, query, variables)
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

	_, err := r.getClient().Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}
}
