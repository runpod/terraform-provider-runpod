package datasource_billing_network_volume

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
	"os"
)

func NewBillingNetworkVolumeDataSource() datasource.DataSource {
	return &BillingNetworkVolumeDataSource{}
}

type BillingNetworkVolumeDataSource struct{}

func (d *BillingNetworkVolumeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_billing_network_volume"
}

func (d *BillingNetworkVolumeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = BillingNetworkVolumeDataSourceSchema(ctx)
}

func (d *BillingNetworkVolumeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config BillingNetworkVolumeModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	params := make(map[string]string)

	if !config.BucketSize.IsNull() {
		params["bucketSize"] = config.BucketSize.ValueString()
	}
	if !config.EndTime.IsNull() {
		params["endTime"] = config.EndTime.ValueString()
	}
	if !config.NetworkVolumeId.IsNull() {
		params["networkVolumeId"] = config.NetworkVolumeId.ValueString()
	}

	apiKey := os.Getenv("RUNPOD_API_KEY")
	if apiKey == "" {
		resp.Diagnostics.AddError("Missing API Key", "RunPod API key is required. Set it via provider config or RUNPOD_API_KEY environment variable.")
		return
	}

	restUrl := os.Getenv("RUNPOD_BASE_URL")
	if restUrl == "" {
		restUrl = client.GetRestBaseURL()
	}

	runpodClient := client.NewRunPodClient(apiKey, "https://api.runpod.io/graphql", restUrl)
	result, err := runpodClient.RestQuery(ctx, "GET", "billing/networkvolumes", params)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if recordsArray, ok := result["records"].([]interface{}); ok {
		records := recordsArray
		models := make([]BillingRecordModel, len(records))
		for i, record := range records {
			if recordMap, ok := record.(map[string]interface{}); ok {
				models[i] = BillingRecordModel{
					Amount: types.Float64Value(recordMap["totalAmount"].(float64)),
					DiskSpaceBilledGb: func() types.Int64 {
						if val, ok := recordMap["diskSpaceBilledGb"].(float64); ok {
							return types.Int64Value(int64(val))
						}
						return types.Int64Null()
					}(),
					NetworkVolumeId: func() types.String {
						if val, ok := recordMap["networkVolumeId"].(string); ok {
							return types.StringValue(val)
						}
						return types.StringNull()
					}(),
					Time: types.StringValue(recordMap["startTime"].(string)),
					TimeBilledMs: func() types.Int64 {
						if val, ok := recordMap["timeBilledMs"].(float64); ok {
							return types.Int64Value(int64(val))
						}
						return types.Int64Null()
					}(),
				}
			}
		}
		model := BillingNetworkVolumeModel{
			BucketSize:        config.BucketSize,
			NetworkVolumeId:   config.NetworkVolumeId,
			BillingRecords:    models,
		}
		diags := resp.State.Set(ctx, &model)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to parse billing records from response")
	}
}
