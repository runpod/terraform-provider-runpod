package datasource_endpoint_workers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/attr"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewEndpointWorkersDataSource() datasource.DataSource {
	return &EndpointWorkersDataSource{}
}

type EndpointWorkersDataSource struct {
	rlClient *client.RunPodClient
}

func (r *EndpointWorkersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EndpointWorkersDataSource) getClient() *client.RunPodClient {
	if r.rlClient != nil {
		return r.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	baseURL := os.Getenv("RUNPOD_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.runpod.io/v2"
	}
	r.rlClient = client.NewRunPodClient(apiKey, "https://api.runpod.io/graphql", baseURL)
	return r.rlClient
}

func (r *EndpointWorkersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_endpoint_workers"
}

func (r *EndpointWorkersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = EndpointWorkersDataSourceSchema(ctx)
}

func (r *EndpointWorkersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config EndpointWorkersDataSourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := r.getClient()

	url := rlClient.RestBaseURL + "/serverless/" + config.EndpointId.ValueString() + "/workers"

	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rlClient.APIKey))

	httpClient := &http.Client{}
	respHTTP, err := httpClient.Do(reqHTTP)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to make API call: %v", err))
		return
	}
	defer respHTTP.Body.Close()

	respBody, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	if respHTTP.StatusCode != 200 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to list workers (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response: %v", err))
		return
	}

	workersData, ok := result["workers"].([]interface{})
	if !ok {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get workers from response: %v", result))
		return
	}

	workers := make([]attr.Value, 0)
	for _, worker := range workersData {
		if workerMap, ok := worker.(map[string]interface{}); ok {
			workerObj := map[string]attr.Value{
				"id":           types.StringValue(workerMap["id"].(string)),
				"pod_id":       types.StringValue(workerMap["podId"].(string)),
				"status":       types.StringValue(workerMap["status"].(string)),
				"uptime_ms":    types.Int64Value(int64(workerMap["uptimeMs"].(float64))),
				"start_time":   types.StringValue(workerMap["startTime"].(string)),
				"last_busy_ms": types.Int64Value(int64(workerMap["lastBusyMs"].(float64))),
			}
			workerVal, diags := types.ObjectValue(map[string]attr.Type{
				"id":           types.StringType,
				"pod_id":       types.StringType,
				"status":       types.StringType,
				"uptime_ms":    types.Int64Type,
				"start_time":   types.StringType,
				"last_busy_ms": types.Int64Type,
			}, workerObj)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			workers = append(workers, workerVal)
		}
	}

	workersList, diags := types.ListValue(types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":           types.StringType,
			"pod_id":       types.StringType,
			"status":       types.StringType,
			"uptime_ms":    types.Int64Type,
			"start_time":   types.StringType,
			"last_busy_ms": types.Int64Type,
		},
	}, workers)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	config.Workers = workersList

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}
