package datasource_data_centers

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewDataCentersDataSource() datasource.DataSource {
	return &DataCentersDataSource{}
}

type DataCentersDataSource struct {
	client *client.RunPodClient
}

func (d *DataCentersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *DataCentersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_data_centers"
}

func (d *DataCentersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = DataCentersDataSourceSchema(ctx)
}

func (d *DataCentersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	query := `
		query GetDataCenters {
			dataCenter {
				id
				name
				location
				globalNetwork
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

	if dataCenters, ok := result["dataCenter"].([]interface{}); ok {
		models := make([]DataCentersModel, len(dataCenters))
		for i, dc := range dataCenters {
			if dcMap, ok := dc.(map[string]interface{}); ok {
				models[i] = DataCentersModel{
					Id:            types.StringValue(dcMap["id"].(string)),
					Name:          types.StringValue(dcMap["name"].(string)),
					Location:      types.StringValue(dcMap["location"].(string)),
					GlobalNetwork: types.BoolValue(dcMap["globalNetwork"].(bool)),
				}
			}
		}
		diags := resp.State.Set(ctx, &models)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to parse data centers from response")
	}
}
