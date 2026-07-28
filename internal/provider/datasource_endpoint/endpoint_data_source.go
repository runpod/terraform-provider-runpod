package datasource_endpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewEndpointDataSource() datasource.DataSource {
	return &EndpointDataSource{}
}

type EndpointDataSource struct {
	rlClient *client.RunPodClient
}

func (r *EndpointDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EndpointDataSource) getClient() *client.RunPodClient {
	if r.rlClient != nil {
		return r.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	baseURL := os.Getenv("RUNPOD_BASE_URL")
	if baseURL == "" {
		baseURL = client.GetRestBaseURL()
	}
	r.rlClient = client.NewRunPodClient(apiKey, "https://api.runpod.io/graphql", baseURL)
	return r.rlClient
}

func (r *EndpointDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_endpoint"
}

func (r *EndpointDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = EndpointDataSourceSchema(ctx)
}

func (r *EndpointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config EndpointDataSourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := r.getClient()
	endpointId := config.Id.ValueString()
	url := rlClient.BaseURL() + "/serverless/" + endpointId

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

	if respHTTP.StatusCode == 404 {
		resp.State.RemoveResource(ctx)
		return
	}

	respBody, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if endpoint, ok := data["endpoint"].(map[string]interface{}); ok {
			result = endpoint
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract endpoint from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if result == nil {
		resp.Diagnostics.AddError("API Error", "Empty response from API")
		return
	}

	if id, ok := result["id"].(string); ok {
		config.Id = types.StringValue(id)
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'id' is missing or not a string in endpoint response")
		return
	}

	if val, ok := result["name"].(string); ok {
		config.Name = types.StringValue(val)
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'name' is missing or not a string in endpoint response")
		return
	}

	if val, ok := result["image"].(string); ok {
		config.Image = types.StringValue(val)
	}

	if val, ok := result["gpu"].(map[string]interface{}); ok {
		if pools, ok := val["pools"].([]interface{}); ok {
			poolsStr := make([]string, len(pools))
			for i, p := range pools {
				if s, ok := p.(string); ok {
					poolsStr[i] = s
				}
			}
			config.GpuPools = types.StringValue(poolsStr[0])
		}
		if count, ok := val["count"].(float64); ok {
			config.GpuCount = types.Int64Value(int64(count))
		}
	}

	if val, ok := result["workers"].(map[string]interface{}); ok {
		if min, ok := val["min"].(float64); ok {
			config.WorkersMin = types.Int64Value(int64(min))
		}
		if max, ok := val["max"].(float64); ok {
			config.WorkersMax = types.Int64Value(int64(max))
		}
	}

	if val, ok := result["scaling"].(map[string]interface{}); ok {
		if typeVal, ok := val["type"].(string); ok {
			config.ScalerType = types.StringValue(typeVal)
		}
		if value, ok := val["value"].(float64); ok {
			config.ScalerValue = types.Float64Value(value)
		}
		if idleTimeout, ok := val["idleTimeout"].(float64); ok {
			config.IdleTimeout = types.Int64Value(int64(idleTimeout))
		}
	}

	if val, ok := result["requestUrls"].(map[string]interface{}); ok {
		if run, ok := val["run"].(string); ok {
			config.RunUrl = types.StringValue(run)
		}
		if runSync, ok := val["runSync"].(string); ok {
			config.RunSyncUrl = types.StringValue(runSync)
		}
	}

	if val, ok := result["createdAt"].(string); ok {
		config.CreatedAt = types.StringValue(val)
	}

	if val, ok := result["dataCenterIds"].([]interface{}); ok {
		if len(val) > 0 {
			if s, ok := val[0].(string); ok {
				config.DataCenterIds = types.StringValue(s)
			}
		}
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}
