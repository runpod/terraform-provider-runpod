package datasource_machines

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewMachinesDataSource() datasource.DataSource {
	return &MachinesDataSource{}
}

type MachinesDataSource struct {
	rlClient *client.RunPodClient
}

func (d *MachinesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *MachinesDataSource) getClient() *client.RunPodClient {
	if d.rlClient != nil {
		return d.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	graphqlEndpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if graphqlEndpoint == "" {
		graphqlEndpoint = "https://api.runpod.io/graphql"
	}
	d.rlClient = client.NewRunPodClient(apiKey, graphqlEndpoint, "")
	return d.rlClient
}

func (d *MachinesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_machines"
}

func (d *MachinesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = MachinesDataSourceSchema(ctx)
}

func (d *MachinesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	query := `
		query GetMachines {
			machines {
				id
				name
				location
				listed
				gpuType {
					id
					displayName
				}
				gpuTotal
				secureCloud
				dataCenterId
			}
		}
	`

	variables := map[string]interface{}{}

	result, err := d.getClient().Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if machines, ok := result["machines"].([]interface{}); ok {
		models := make([]MachinesModel, len(machines))
		for i, machine := range machines {
			if machineMap, ok := machine.(map[string]interface{}); ok {
				gpuTypeMap, ok := machineMap["gpuType"].(map[string]interface{})
				gpuTypeId := types.StringValue("")
				if ok {
					if id, ok := gpuTypeMap["id"].(string); ok {
						gpuTypeId = types.StringValue(id)
					}
				}
				
				var id, name, location, dataCenterId string
				var listed bool
				var gpuTotal float64
				var secureCloud bool
				
				if v, ok := machineMap["id"].(string); ok {
					id = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'id' is missing or not a string in machine response")
					return
				}
				
				if v, ok := machineMap["name"].(string); ok {
					name = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'name' is missing or not a string in machine response")
					return
				}
				
				if v, ok := machineMap["location"].(string); ok {
					location = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'location' is missing or not a string in machine response")
					return
				}
				
				if v, ok := machineMap["listed"].(bool); ok {
					listed = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'listed' is missing or not a bool in machine response")
					return
				}
				
				if v, ok := machineMap["gpuTotal"].(float64); ok {
					gpuTotal = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'gpuTotal' is missing or not a float64 in machine response")
					return
				}
				
				if v, ok := machineMap["secureCloud"].(bool); ok {
					secureCloud = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'secureCloud' is missing or not a bool in machine response")
					return
				}
				
				if v, ok := machineMap["dataCenterId"].(string); ok {
					dataCenterId = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'dataCenterId' is missing or not a string in machine response")
					return
				}
				
				models[i] = MachinesModel{
					Id:           types.StringValue(id),
					Name:         types.StringValue(name),
					Location:     types.StringValue(location),
					Listed:       types.BoolValue(listed),
					GpuTypeId:    gpuTypeId,
					GpuTotal:     types.Int64Value(int64(gpuTotal)),
					SecureCloud:  types.BoolValue(secureCloud),
					DataCenterId: types.StringValue(dataCenterId),
				}
			}
		}
		parent := MachinesDataSourceModel{
			Machines: models,
		}
		diags := resp.State.Set(ctx, &parent)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to parse machines from response")
	}
}
