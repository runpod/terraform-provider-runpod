package datasource_data_centers

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
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

func (d *DataCentersDataSource) getClient() *client.RunPodClient {
	if d.client != nil {
		return d.client
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	graphqlEndpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if graphqlEndpoint == "" {
		graphqlEndpoint = "https://api.runpod.io/graphql"
	}
	d.client = client.NewRunPodClient(apiKey, graphqlEndpoint, "https://rest.runpod.io/v1")
	return d.client
}

func (d *DataCentersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_data_centers"
}

func (d *DataCentersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = DataCentersDataSourceSchema(ctx)
}

func (d *DataCentersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Read input arguments (gpu_count / secure_cloud) that parameterize the
	// per-data-center GPU availability query.
	var config DataCentersDataSourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	gpuCount := int64(1)
	if !config.GpuCount.IsNull() && config.GpuCount.ValueInt64() > 0 {
		gpuCount = config.GpuCount.ValueInt64()
	}
	secureCloud := true
	if !config.SecureCloud.IsNull() {
		secureCloud = config.SecureCloud.ValueBool()
	}

	// gpuAvailability(input:) is inlined as an object literal (matching the form
	// verified against the real RunPod GraphQL API) to avoid depending on the
	// server-side input type name. gpuCount/secureCloud are validated scalars.
	query := fmt.Sprintf(`
		query GetDataCenters {
			dataCenters {
				id
				name
				location
				globalNetwork
				gpuAvailability(input: { gpuCount: %d, secureCloud: %t }) {
					gpuTypeId
					displayName
					stockStatus
					available
				}
			}
		}
	`, gpuCount, secureCloud)

	variables := map[string]interface{}{}

	result, err := d.getClient().Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if dataCenters, ok := result["dataCenters"].([]interface{}); ok {
		models := make([]DataCentersModel, len(dataCenters))
		for i, dc := range dataCenters {
			if dcMap, ok := dc.(map[string]interface{}); ok {
				var id, name, location string
				var globalNetwork bool

				if v, ok := dcMap["id"].(string); ok {
					id = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'id' is missing or not a string in data center response")
					return
				}

				if v, ok := dcMap["name"].(string); ok {
					name = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'name' is missing or not a string in data center response")
					return
				}

				if v, ok := dcMap["location"].(string); ok {
					location = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'location' is missing or not a string in data center response")
					return
				}

				if v, ok := dcMap["globalNetwork"].(bool); ok {
					globalNetwork = v
				} else {
					resp.Diagnostics.AddError("API Error", "Field 'globalNetwork' is missing or not a bool in data center response")
					return
				}

				// gpuAvailability is optional/nullable per data center.
				availability := make([]GpuAvailabilityModel, 0)
				if rawAvail, ok := dcMap["gpuAvailability"].([]interface{}); ok {
					for _, ga := range rawAvail {
						gaMap, ok := ga.(map[string]interface{})
						if !ok {
							continue
						}

						gpuTypeId := ""
						if v, ok := gaMap["gpuTypeId"].(string); ok {
							gpuTypeId = v
						}
						displayName := ""
						if v, ok := gaMap["displayName"].(string); ok {
							displayName = v
						}
						stockStatus := ""
						if v, ok := gaMap["stockStatus"].(string); ok {
							stockStatus = v
						}
						available := false
						if v, ok := gaMap["available"].(bool); ok {
							available = v
						}

						availability = append(availability, GpuAvailabilityModel{
							GpuTypeId:   types.StringValue(gpuTypeId),
							DisplayName: types.StringValue(displayName),
							StockStatus: types.StringValue(stockStatus),
							Available:   types.BoolValue(available),
						})
					}
				}

				models[i] = DataCentersModel{
					Id:              types.StringValue(id),
					Name:            types.StringValue(name),
					Location:        types.StringValue(location),
					GlobalNetwork:   types.BoolValue(globalNetwork),
					GpuAvailability: availability,
				}
			}
		}
		parent := DataCentersDataSourceModel{
			GpuCount:    config.GpuCount,
			SecureCloud: config.SecureCloud,
			DataCenters: models,
		}
		diags := resp.State.Set(ctx, &parent)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to parse data centers from response")
	}
}
