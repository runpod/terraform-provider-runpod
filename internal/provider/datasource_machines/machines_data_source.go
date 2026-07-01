package datasource_machines

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewMachinesDataSource() datasource.DataSource {
	return &MachinesDataSource{}
}

type MachinesDataSource struct {
	client *client.RunPodClient
}

func (d *MachinesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.RunPodClient)
	}
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
				gpuType
				gpuTotal
				secureCloud
				dataCenterId
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

	if machines, ok := result["machines"].([]interface{}); ok {
		models := make([]MachinesModel, len(machines))
		for i, machine := range machines {
			if machineMap, ok := machine.(map[string]interface{}); ok {
				models[i] = MachinesModel{
					Id:           types.StringValue(machineMap["id"].(string)),
					Name:         types.StringValue(machineMap["name"].(string)),
					Location:     types.StringValue(machineMap["location"].(string)),
					Listed:       types.BoolValue(machineMap["listed"].(bool)),
					GpuTypeId:    types.StringValue(machineMap["gpuType"].(string)),
					GpuTotal:     types.Int64Value(int64(machineMap["gpuTotal"].(float64))),
					SecureCloud:  types.BoolValue(machineMap["secureCloud"].(bool)),
					DataCenterId: types.StringValue(machineMap["dataCenterId"].(string)),
				}
			}
		}
		diags := resp.State.Set(ctx, &models)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to parse machines from response")
	}
}
