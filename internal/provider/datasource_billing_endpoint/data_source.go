package datasource_billing_endpoint

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
	"os"
)

func NewBillingEndpointDataSource() datasource.DataSource {
	return &BillingEndpointDataSource{}
}

type BillingEndpointDataSource struct{}

func (d *BillingEndpointDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_billing_endpoint"
}

func (d *BillingEndpointDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = BillingEndpointDataSourceSchema(ctx)
}

func (d *BillingEndpointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config BillingEndpointModel
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
	if !config.EndpointId.IsNull() {
		params["endpointId"] = config.EndpointId.ValueString()
	}

	apiKey := os.Getenv("RUNPOD_API_KEY")
	if apiKey == "" {
		resp.Diagnostics.AddError("Missing API Key", "RunPod API key is required. Set it via provider config or RUNPOD_API_KEY environment variable.")
		return
	}

	restUrl := os.Getenv("RUNPOD_BASE_URL")
	if restUrl == "" {
		restUrl = "https://rest.runpod.io/v1"
	}

	runpodClient := client.NewRunPodClient(apiKey, restUrl)
	result, err := runpodClient.RestQuery(ctx, "GET", "billing/endpoints", params)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if billingArray, ok := result["billing"].([]interface{}); ok {
		records := billingArray
		models := make([]BillingRecordModel, len(records))
		for i, record := range records {
			if recordMap, ok := record.(map[string]interface{}); ok {
				models[i] = BillingRecordModel{
					Amount: types.Float64Value(recordMap["amount"].(float64)),
					EndpointId: func() types.String {
						if val, ok := recordMap["endpointId"].(string); ok {
							return types.StringValue(val)
						}
						return types.StringNull()
					}(),
					Time: types.StringValue(recordMap["time"].(string)),
					TimeBilledMs: func() types.Int64 {
						if val, ok := recordMap["timeBilledMs"].(float64); ok {
							return types.Int64Value(int64(val))
						}
						return types.Int64Null()
					}(),
				}
			}
		}
		diags := resp.State.Set(ctx, &models)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to parse billing records from response")
	}
}
